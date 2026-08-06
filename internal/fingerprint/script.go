package fingerprint

import (
	"encoding/json"
	"fmt"
)

type scriptConfig struct {
	Seed                string   `json:"seed"`
	Locale              string   `json:"locale"`
	Languages           []string `json:"languages"`
	NavigatorPlatform   string   `json:"navigatorPlatform"`
	HardwareConcurrency int64    `json:"hardwareConcurrency"`
	DeviceMemory        int      `json:"deviceMemory"`
	WebGLVendor         string   `json:"webglVendor"`
	WebGLRenderer       string   `json:"webglRenderer"`
}

// BuildScript returns an idempotent script. CDP installs it with
// Page.addScriptToEvaluateOnNewDocument before navigation and runs it in the
// current about:blank document as well.
func BuildScript(identity Identity) (string, error) {
	languages := []string{identity.Locale}
	if identity.Locale != "en-US" {
		languages = append(languages, "en-US", "en")
	} else {
		languages = append(languages, "en")
	}
	configuration, err := json.Marshal(scriptConfig{
		Seed: identity.Seed, Locale: identity.Locale, Languages: languages,
		NavigatorPlatform:   identity.NavigatorPlatform,
		HardwareConcurrency: identity.HardwareConcurrency, DeviceMemory: identity.DeviceMemory,
		WebGLVendor: identity.WebGLVendor, WebGLRenderer: identity.WebGLRenderer,
	})
	if err != nil {
		return "", fmt.Errorf("encode fingerprint script configuration: %w", err)
	}

	return fmt.Sprintf(`(() => {
  "use strict";
  const cfg = %s;
  const installed = Symbol.for("bruno.browser.fingerprint.v1");
  if (globalThis[installed]) return;
  Object.defineProperty(globalThis, installed, { value: true, configurable: false });

  const nativeSources = new WeakMap();
  const originalToString = Function.prototype.toString;
  const remember = (replacement, original) => {
    try { nativeSources.set(replacement, originalToString.call(original)); } catch (_) {}
    return replacement;
  };
  const maskedToString = remember(function toString() {
    return nativeSources.get(this) || originalToString.call(this);
  }, originalToString);
  Object.defineProperty(Function.prototype, "toString", {
    value: maskedToString, configurable: true, writable: true
  });

  const defineGetter = (object, name, getter) => {
    if (!object) return;
    const descriptor = Object.getOwnPropertyDescriptor(object, name);
    try {
      Object.defineProperty(object, name, {
        get: remember(getter, descriptor && descriptor.get ? descriptor.get : getter),
        configurable: descriptor ? descriptor.configurable : true,
        enumerable: descriptor ? descriptor.enumerable : false
      });
    } catch (_) {}
  };
  const replaceMethod = (prototype, name, factory) => {
    if (!prototype || typeof prototype[name] !== "function") return;
    const original = prototype[name];
    const replacement = remember(factory(original), original);
    try { Object.defineProperty(prototype, name, { value: replacement, configurable: true, writable: true }); } catch (_) {}
  };
  const hash = (label, index) => {
    let value = 2166136261 >>> 0;
    const input = cfg.seed + ":" + label + ":" + index;
    for (let i = 0; i < input.length; i++) {
      value ^= input.charCodeAt(i);
      value = Math.imul(value, 16777619) >>> 0;
    }
    return value >>> 0;
  };
  const signedNoise = (label, index) => (hash(label, index) %% 3) - 1;

  const navigatorPrototype = globalThis.Navigator && Navigator.prototype;
  defineGetter(navigatorPrototype, "webdriver", function webdriver() { return undefined; });
  defineGetter(navigatorPrototype, "language", function language() { return cfg.locale; });
  defineGetter(navigatorPrototype, "languages", function languages() { return Object.freeze(cfg.languages.slice()); });
  defineGetter(navigatorPrototype, "platform", function platform() { return cfg.navigatorPlatform; });
  defineGetter(navigatorPrototype, "hardwareConcurrency", function hardwareConcurrency() { return cfg.hardwareConcurrency; });
  defineGetter(navigatorPrototype, "deviceMemory", function deviceMemory() { return cfg.deviceMemory; });

  const permissionsPrototype = globalThis.Permissions && Permissions.prototype;
  replaceMethod(permissionsPrototype, "query", original => async function query(parameters) {
    if (parameters && parameters.name === "notifications" && globalThis.Notification) {
      return { state: Notification.permission, onchange: null };
    }
    return original.apply(this, arguments);
  });

  const perturbPixels = (data, label) => {
    if (!data || typeof data.length !== "number") return data;
    const limit = Math.min(data.length, 128);
    for (let i = 0; i < limit; i += 4) {
      const channel = hash(label, i) %% 3;
      const position = i + channel;
      if (position < data.length) data[position] = Math.max(0, Math.min(255, data[position] + signedNoise(label, position)));
    }
    return data;
  };

  const canvas2D = globalThis.CanvasRenderingContext2D && CanvasRenderingContext2D.prototype;
  replaceMethod(canvas2D, "getImageData", original => function getImageData() {
    const image = original.apply(this, arguments);
    perturbPixels(image && image.data, "canvas-image-data");
    return image;
  });

  const canvasPrototype = globalThis.HTMLCanvasElement && HTMLCanvasElement.prototype;
  const cloneCanvas = canvas => {
    try {
      if (!canvas.width || !canvas.height) return canvas;
      const clone = document.createElement("canvas");
      clone.width = canvas.width;
      clone.height = canvas.height;
      const context = clone.getContext("2d");
      if (!context) return canvas;
      context.drawImage(canvas, 0, 0);
      const x = hash("canvas-x", canvas.width) %% canvas.width;
      const y = hash("canvas-y", canvas.height) %% canvas.height;
      const pixel = context.getImageData(x, y, 1, 1);
      perturbPixels(pixel.data, "canvas-export");
      context.putImageData(pixel, x, y);
      return clone;
    } catch (_) { return canvas; }
  };
  replaceMethod(canvasPrototype, "toDataURL", original => function toDataURL() {
    return original.apply(cloneCanvas(this), arguments);
  });
  replaceMethod(canvasPrototype, "toBlob", original => function toBlob() {
    return original.apply(cloneCanvas(this), arguments);
  });

  const patchWebGL = prototype => {
    replaceMethod(prototype, "getParameter", original => function getParameter(parameter) {
      if (parameter === 37445) return cfg.webglVendor;
      if (parameter === 37446) return cfg.webglRenderer;
      return original.apply(this, arguments);
    });
    replaceMethod(prototype, "readPixels", original => function readPixels() {
      const result = original.apply(this, arguments);
      perturbPixels(arguments[6], "webgl-read-pixels");
      return result;
    });
  };
  patchWebGL(globalThis.WebGLRenderingContext && WebGLRenderingContext.prototype);
  patchWebGL(globalThis.WebGL2RenderingContext && WebGL2RenderingContext.prototype);

  const touchedAudioArrays = new WeakSet();
  const perturbAudio = (array, label) => {
    if (!array || touchedAudioArrays.has(array)) return array;
    touchedAudioArrays.add(array);
    const limit = Math.min(array.length, 96);
    for (let i = 0; i < limit; i += 8) {
      if (typeof array[i] === "number") array[i] += signedNoise(label, i) * 1e-7;
    }
    return array;
  };
  const audioBufferPrototype = globalThis.AudioBuffer && AudioBuffer.prototype;
  replaceMethod(audioBufferPrototype, "getChannelData", original => function getChannelData() {
    return perturbAudio(original.apply(this, arguments), "audio-buffer");
  });
  replaceMethod(audioBufferPrototype, "copyFromChannel", original => function copyFromChannel() {
    const result = original.apply(this, arguments);
    perturbAudio(arguments[0], "audio-copy");
    return result;
  });
  const analyserPrototype = globalThis.AnalyserNode && AnalyserNode.prototype;
  for (const method of ["getFloatFrequencyData", "getFloatTimeDomainData"]) {
    replaceMethod(analyserPrototype, method, original => function analyserFloatData() {
      const result = original.apply(this, arguments);
      perturbAudio(arguments[0], "audio-analyser-" + method);
      return result;
    });
  }
})();`, configuration), nil
}
