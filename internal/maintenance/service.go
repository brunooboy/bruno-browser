package maintenance

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"bruno-browser/internal/domain"
	"bruno-browser/internal/profile"
)

var ErrProfileRunning = errors.New("profile must be closed before maintenance")

type Operation string

const (
	OperationDeleteProfile        Operation = "delete_profile"
	OperationClearHistoryAndCache Operation = "clear_history_and_cache"
	OperationClearCookiesSession  Operation = "clear_cookies_session"
)

type Report struct {
	ProfileID      string    `json:"profileId"`
	Operation      Operation `json:"operation"`
	RemovedTargets []string  `json:"removedTargets"`
	BytesFreed     int64     `json:"bytesFreed"`
}

type ProfileStore interface {
	Get(context.Context, string) (domain.Metadata, error)
	Paths(string) (profile.Paths, error)
	Delete(context.Context, string) error
}

type ProcessState interface {
	BeginMaintenance(string) (release func(), ok bool)
}

type Service struct {
	profiles  ProfileStore
	processes ProcessState
	locks     keyedLock
}

func NewService(profiles ProfileStore, processes ProcessState) (*Service, error) {
	if profiles == nil {
		return nil, errors.New("profile store is required")
	}
	if processes == nil {
		return nil, errors.New("browser process state is required")
	}
	return &Service{profiles: profiles, processes: processes}, nil
}

func (s *Service) DeleteProfile(ctx context.Context, profileID string) (Report, error) {
	metadata, paths, unlock, err := s.begin(ctx, profileID)
	if err != nil {
		return Report{}, err
	}
	defer unlock()

	bytesFreed, err := measurePath(ctx, paths.Root)
	if err != nil {
		return Report{}, fmt.Errorf("measure profile before deletion: %w", err)
	}
	if err := s.profiles.Delete(ctx, metadata.ID); err != nil {
		return Report{}, fmt.Errorf("delete profile: %w", err)
	}
	return Report{
		ProfileID:      metadata.ID,
		Operation:      OperationDeleteProfile,
		RemovedTargets: []string{"."},
		BytesFreed:     bytesFreed,
	}, nil
}

func (s *Service) ClearHistoryAndCache(ctx context.Context, profileID string) (Report, error) {
	return s.clear(ctx, profileID, OperationClearHistoryAndCache, historyAndCacheTargets)
}

func (s *Service) ClearCookiesAndSession(ctx context.Context, profileID string) (Report, error) {
	return s.clear(ctx, profileID, OperationClearCookiesSession, cookiesAndSessionTargets)
}

func (s *Service) clear(ctx context.Context, profileID string, operation Operation, targets []string) (Report, error) {
	metadata, paths, unlock, err := s.begin(ctx, profileID)
	if err != nil {
		return Report{}, err
	}
	defer unlock()

	if err := validateDirectory(paths.UserData); err != nil {
		return Report{}, fmt.Errorf("validate Chromium user-data directory: %w", err)
	}
	report := Report{ProfileID: metadata.ID, Operation: operation}
	for _, relativeTarget := range targets {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		removed, bytesFreed, err := removeRelativeTarget(ctx, paths.UserData, relativeTarget)
		if err != nil {
			return report, fmt.Errorf("remove %q: %w", relativeTarget, err)
		}
		if removed {
			report.RemovedTargets = append(report.RemovedTargets, filepath.ToSlash(relativeTarget))
			report.BytesFreed += bytesFreed
		}
	}
	return report, nil
}

func (s *Service) begin(ctx context.Context, profileID string) (domain.Metadata, profile.Paths, func(), error) {
	if err := ctx.Err(); err != nil {
		return domain.Metadata{}, profile.Paths{}, nil, err
	}
	paths, err := s.profiles.Paths(profileID)
	if err != nil {
		return domain.Metadata{}, profile.Paths{}, nil, err
	}
	unlock := s.locks.lock(paths.Root)
	metadata, err := s.profiles.Get(ctx, profileID)
	if err != nil {
		unlock()
		return domain.Metadata{}, profile.Paths{}, nil, err
	}
	releaseProcess, ok := s.processes.BeginMaintenance(metadata.ID)
	if !ok {
		unlock()
		return domain.Metadata{}, profile.Paths{}, nil, ErrProfileRunning
	}
	release := func() {
		releaseProcess()
		unlock()
	}
	return metadata, paths, release, nil
}

func validateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic-link directories are not allowed")
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return nil
}

func removeRelativeTarget(ctx context.Context, base, relativeTarget string) (bool, int64, error) {
	target, err := safeTargetPath(base, relativeTarget)
	if err != nil {
		return false, 0, err
	}
	if err := validateParentChain(base, filepath.Dir(target)); err != nil {
		return false, 0, err
	}
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return false, 0, nil
	} else if err != nil {
		return false, 0, err
	}
	bytesFreed, err := measurePath(ctx, target)
	if err != nil {
		return false, 0, err
	}
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	if err := os.RemoveAll(target); err != nil {
		return false, 0, err
	}
	return true, bytesFreed, nil
}

func safeTargetPath(base, relativeTarget string) (string, error) {
	cleanTarget := filepath.Clean(filepath.FromSlash(relativeTarget))
	if cleanTarget == "." || filepath.IsAbs(cleanTarget) {
		return "", errors.New("maintenance target must be a relative child path")
	}
	target := filepath.Join(base, cleanTarget)
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("maintenance target escapes the user-data directory")
	}
	return target, nil
}

func validateParentChain(base, parent string) error {
	relative, err := filepath.Rel(base, parent)
	if err != nil {
		return err
	}
	current := base
	if relative == "." {
		return validateDirectory(base)
	}
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic-link parent %q is not allowed", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("parent %q is not a directory", current)
		}
	}
	return nil
}

func measurePath(ctx context.Context, path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}
