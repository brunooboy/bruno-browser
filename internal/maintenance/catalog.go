package maintenance

// historyAndCacheTargets deliberately excludes Cookies, Login Data,
// Local Storage, IndexedDB and session files so authenticated accounts remain
// signed in after a selective cleanup.
var historyAndCacheTargets = []string{
	"Cache",
	"Code Cache",
	"DawnCache",
	"GPUCache",
	"GrShaderCache",
	"GraphiteDawnCache",
	"ShaderCache",
	"Default/Cache",
	"Default/Code Cache",
	"Default/DawnCache",
	"Default/GPUCache",
	"Default/Media Cache",
	"Default/Service Worker/CacheStorage",
	"Default/Service Worker/ScriptCache",
	"Default/Shared Dictionary/cache",
	"Default/History",
	"Default/History-journal",
	"Default/History-shm",
	"Default/History-wal",
	"Default/Shortcuts",
	"Default/Shortcuts-journal",
	"Default/Top Sites",
	"Default/Top Sites-journal",
	"Default/Visited Links",
}

// cookiesAndSessionTargets resets site authentication while preserving the
// profile directory, Preferences, bookmarks, extensions and saved passwords.
var cookiesAndSessionTargets = []string{
	"Default/Cookies",
	"Default/Cookies-journal",
	"Default/Cookies-shm",
	"Default/Cookies-wal",
	"Default/Network/Cookies",
	"Default/Network/Cookies-journal",
	"Default/Network/Cookies-shm",
	"Default/Network/Cookies-wal",
	"Default/Current Session",
	"Default/Current Tabs",
	"Default/Last Session",
	"Default/Last Tabs",
	"Default/Sessions",
	"Default/Session Storage",
	"Default/Local Storage",
	"Default/IndexedDB",
	"Default/WebStorage",
	"Default/Storage",
	"Default/Service Worker",
	"Default/SharedStorage",
	"Default/SharedStorage-shm",
	"Default/SharedStorage-wal",
}
