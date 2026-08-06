package fingerprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// URLRecorder avoids coupling the CDP controller to the profile package's
// concrete return type.
type URLRecorder func(context.Context, string, string) error

type Controller struct {
	store  *Store
	record URLRecorder
}

type Health struct {
	StandardReady bool   `json:"standardReady"`
	WayfernReady  bool   `json:"wayfernReady"`
	Error         string `json:"error,omitempty"`
}

func (controller *Controller) Health(ctx context.Context, profileID string) Health {
	var health Health
	if _, exists, err := controller.store.Inspect(ctx, profileID); err != nil {
		health.Error = err.Error()
	} else {
		health.StandardReady = exists
	}
	if _, exists, err := controller.store.LoadWayfern(ctx, profileID); err != nil {
		if health.Error == "" {
			health.Error = err.Error()
		}
	} else {
		health.WayfernReady = exists
	}
	return health
}

func NewController(store *Store, recorder URLRecorder) (*Controller, error) {
	if store == nil {
		return nil, errors.New("fingerprint store is required")
	}
	if recorder == nil {
		return nil, errors.New("last URL recorder is required")
	}
	return &Controller{store: store, record: recorder}, nil
}

type Session struct {
	profileID string
	client    *cdpClient
	identity  Identity
	script    string
	recorder  *navigationRecorder

	mu         sync.Mutex
	protecting map[string]struct{}
	closed     bool
	lastError  error
	wg         sync.WaitGroup
	closeOnce  sync.Once
}

type browserVersionResult struct {
	Product   string `json:"product"`
	UserAgent string `json:"userAgent"`
}

type targetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	URL      string `json:"url"`
}

func (controller *Controller) Attach(ctx context.Context, profileID, websocketURL, initialURL string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(websocketURL) == "" {
		return nil, errors.New("DevTools websocket URL is required")
	}
	if !restorableURL(initialURL) && initialURL != "about:blank" {
		return nil, errors.New("initial page must use http or https")
	}
	stored, err := controller.store.LoadOrCreate(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("load persistent fingerprint: %w", err)
	}

	client, err := newCDPClient(ctx, websocketURL)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = client.Close()
		}
	}()
	operationContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var version browserVersionResult
	if err := client.Call(operationContext, "", "Browser.getVersion", map[string]any{}, &version); err != nil {
		return nil, fmt.Errorf("read Chromium version: %w", err)
	}
	identity, err := BuildIdentity(stored, version.Product, version.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("build runtime fingerprint: %w", err)
	}
	script, err := BuildScript(identity)
	if err != nil {
		return nil, err
	}

	var targets struct {
		TargetInfos []targetInfo `json:"targetInfos"`
	}
	if err := client.Call(operationContext, "", "Target.getTargets", map[string]any{}, &targets); err != nil {
		return nil, fmt.Errorf("list Chromium targets: %w", err)
	}
	rootTargetID := ""
	for _, candidate := range targets.TargetInfos {
		if candidate.Type == "page" {
			rootTargetID = candidate.TargetID
			if candidate.URL == "about:blank" || candidate.URL == "" {
				break
			}
		}
	}
	if rootTargetID == "" {
		return nil, errors.New("Chromium did not expose an initial page target")
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := client.Call(operationContext, "", "Target.attachToTarget", map[string]any{
		"targetId": rootTargetID, "flatten": true,
	}, &attached); err != nil {
		return nil, fmt.Errorf("attach initial Chromium target: %w", err)
	}
	if attached.SessionID == "" {
		return nil, errors.New("Chromium returned an empty CDP session id")
	}

	session := &Session{
		profileID: profileID, client: client, identity: identity, script: script,
		recorder: newNavigationRecorder(profileID, controller.record), protecting: make(map[string]struct{}),
	}
	session.wg.Add(1)
	go session.eventLoop()
	if err := session.applyProtection(operationContext, attached.SessionID, "page", false); err != nil {
		session.Close()
		return nil, fmt.Errorf("apply fingerprint to initial page: %w", err)
	}

	// Browser-level auto-attach pauses every future page or worker. The event
	// loop configures the attached session directly, then releases it. This is
	// what guarantees pop-ups and user-created tabs cannot run page code first.
	if err := client.Call(operationContext, "", "Target.setAutoAttach", map[string]any{
		"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true,
	}, nil); err != nil {
		session.Close()
		return nil, fmt.Errorf("enable browser-wide pre-navigation protection: %w", err)
	}
	if initialURL != "" && initialURL != "about:blank" {
		if err := client.Call(operationContext, attached.SessionID, "Page.navigate", map[string]any{"url": initialURL}, nil); err != nil {
			session.Close()
			return nil, fmt.Errorf("restore protected page: %w", err)
		}
	}

	failed = false
	return session, nil
}

func (session *Session) eventLoop() {
	defer session.wg.Done()
	for {
		select {
		case <-session.client.done:
			return
		case event := <-session.client.events:
			switch event.Method {
			case "Target.attachedToTarget":
				var attached struct {
					SessionID          string     `json:"sessionId"`
					TargetInfo         targetInfo `json:"targetInfo"`
					WaitingForDebugger bool       `json:"waitingForDebugger"`
				}
				if json.Unmarshal(event.Params, &attached) == nil && attached.SessionID != "" && attached.WaitingForDebugger {
					session.protectAsync(attached.SessionID, attached.TargetInfo)
				}
			case "Page.frameNavigated":
				var navigation struct {
					Frame struct {
						ParentID    string `json:"parentId"`
						URL         string `json:"url"`
						URLFragment string `json:"urlFragment"`
					} `json:"frame"`
				}
				if json.Unmarshal(event.Params, &navigation) == nil && navigation.Frame.ParentID == "" {
					session.recorder.submit(navigation.Frame.URL + navigation.Frame.URLFragment)
				}
			}
		}
	}
}

func (session *Session) protectAsync(sessionID string, information targetInfo) {
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return
	}
	if _, exists := session.protecting[sessionID]; exists {
		session.mu.Unlock()
		return
	}
	session.protecting[sessionID] = struct{}{}
	session.wg.Add(1)
	session.mu.Unlock()

	go func() {
		defer session.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if err := session.applyProtection(ctx, sessionID, information.Type, true); err != nil {
			session.mu.Lock()
			session.lastError = fmt.Errorf("protect target %s (%s): %w", information.TargetID, information.Type, err)
			session.mu.Unlock()
			if information.TargetID != "" {
				_ = session.client.Call(ctx, "", "Target.closeTarget", map[string]any{"targetId": information.TargetID}, nil)
			}
		}
	}()
}

func (session *Session) applyProtection(ctx context.Context, sessionID, targetType string, resume bool) error {
	metadata := map[string]any{
		"brands": []map[string]string{
			{"brand": "Not=A?Brand", "version": "99"},
			{"brand": "Chromium", "version": session.identity.BrowserMajor},
			{"brand": session.identity.PrimaryBrand, "version": session.identity.BrowserMajor},
		},
		"fullVersionList": []map[string]string{
			{"brand": "Not=A?Brand", "version": "99.0.0.0"},
			{"brand": "Chromium", "version": session.identity.BrowserVersion},
			{"brand": session.identity.PrimaryBrand, "version": session.identity.PrimaryBrandVersion},
		},
		"platform": session.identity.Platform, "platformVersion": session.identity.PlatformVersion,
		"architecture": session.identity.Architecture, "model": "", "mobile": false,
		"bitness": session.identity.Bitness, "wow64": false, "formFactors": []string{"Desktop"},
	}
	var firstErr error
	run := func(method string, params any, result any) {
		if err := session.client.Call(ctx, sessionID, method, params, result); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	run("Runtime.enable", map[string]any{}, nil)
	isPage := targetType == "page" || targetType == "iframe" || targetType == "webview" || targetType == "background_page"
	if isPage {
		run("Page.enable", map[string]any{}, nil)
	}
	run("Emulation.setUserAgentOverride", map[string]any{
		"userAgent": session.identity.UserAgent, "acceptLanguage": session.identity.AcceptLanguage,
		"platform": session.identity.NavigatorPlatform, "userAgentMetadata": metadata,
	}, nil)
	run("Emulation.setTimezoneOverride", map[string]any{"timezoneId": session.identity.Timezone}, nil)
	run("Emulation.setLocaleOverride", map[string]any{"locale": strings.ReplaceAll(session.identity.Locale, "-", "_")}, nil)
	run("Emulation.setHardwareConcurrencyOverride", map[string]any{"hardwareConcurrency": session.identity.HardwareConcurrency}, nil)
	// Older Chromium versions may not expose this experimental command. The
	// document script also masks navigator.webdriver, so this is best-effort.
	_ = session.client.Call(ctx, sessionID, "Emulation.setAutomationOverride", map[string]any{"enabled": false}, nil)
	if isPage {
		run("Page.addScriptToEvaluateOnNewDocument", map[string]any{
			"source": session.script, "runImmediately": true,
		}, nil)
	} else {
		var evaluation struct {
			ExceptionDetails json.RawMessage `json:"exceptionDetails"`
		}
		run("Runtime.evaluate", map[string]any{"expression": session.script}, &evaluation)
		if len(evaluation.ExceptionDetails) > 0 && firstErr == nil {
			firstErr = errors.New("fingerprint script failed inside worker target")
		}
	}
	run("Target.setAutoAttach", map[string]any{
		"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true,
	}, nil)
	// Always release a paused target. If a critical protection failed, the
	// caller closes that target immediately rather than letting it run bare.
	if resume {
		if err := session.client.Call(ctx, sessionID, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (session *Session) LastProtectionError() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.lastError
}

// Shutdown asks Chromium to close cleanly so its session databases and last
// visited pages are flushed before the process exits.
func (session *Session) Shutdown(ctx context.Context) error {
	return session.client.Call(ctx, "", "Browser.close", map[string]any{}, nil)
}

func (session *Session) Close() error {
	var closeErr error
	session.closeOnce.Do(func() {
		session.mu.Lock()
		session.closed = true
		session.mu.Unlock()
		session.recorder.close()
		closeErr = session.client.Close()
		session.wg.Wait()
	})
	return closeErr
}

type navigationRecorder struct {
	profileID string
	record    URLRecorder
	urls      chan string
	stop      chan struct{}
	done      chan struct{}
	once      sync.Once
}

func newNavigationRecorder(profileID string, record URLRecorder) *navigationRecorder {
	recorder := &navigationRecorder{
		profileID: profileID, record: record, urls: make(chan string, 32), stop: make(chan struct{}), done: make(chan struct{}),
	}
	go recorder.run()
	return recorder
}

func (recorder *navigationRecorder) submit(rawURL string) {
	if !restorableURL(rawURL) {
		return
	}
	select {
	case <-recorder.stop:
		return
	default:
	}
	select {
	case <-recorder.stop:
		return
	case recorder.urls <- rawURL:
	default:
		select {
		case <-recorder.urls:
		default:
		}
		select {
		case recorder.urls <- rawURL:
		default:
		}
	}
}

func (recorder *navigationRecorder) run() {
	defer close(recorder.done)
	last := ""
	for {
		select {
		case <-recorder.stop:
			finalURL := last
			for {
				select {
				case rawURL := <-recorder.urls:
					finalURL = rawURL
				default:
					if finalURL != "" && finalURL != last {
						_ = recorder.record(context.Background(), recorder.profileID, finalURL)
					}
					return
				}
			}
		case rawURL := <-recorder.urls:
			if rawURL == last {
				continue
			}
			last = rawURL
			_ = recorder.record(context.Background(), recorder.profileID, rawURL)
		}
	}
}

func (recorder *navigationRecorder) close() {
	recorder.once.Do(func() {
		close(recorder.stop)
		<-recorder.done
	})
}

func restorableURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
