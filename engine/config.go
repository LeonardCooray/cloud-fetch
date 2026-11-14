package engine

type Config struct {
	AutoStart         bool
	DisableEncryption bool
	DownloadDirectory string
	EnableUpload      bool
	EnableSeeding     bool
	IncomingPort      int
	// below this much free space in the download directory, downloads are
	// held (see Engine.FreeSpace); 0 turns the guard off
	MinimumFreeSpaceGB int
	// top-level downloads untouched this many days are deleted (see
	// Engine.CleanupDownloads); 0 turns it off
	DeleteDownloadsAfterDays int
	// caps on the client's total peer traffic in KB/s (1000 bytes, as the
	// web UI shows rates); 0 is unlimited
	MaxDownloadKBps int
	MaxUploadKBps   int
	// unfinished torrents beyond this many wait their turn (see
	// Engine.queueLocked); 0 is no limit
	MaxActiveTorrents int
}

// sameClient reports whether a and b differ only in settings the engine
// applies itself, so switching between them needs no client restart.
func sameClient(a, b Config) bool {
	a.MinimumFreeSpaceGB, b.MinimumFreeSpaceGB = 0, 0
	a.DeleteDownloadsAfterDays, b.DeleteDownloadsAfterDays = 0, 0
	a.MaxDownloadKBps, b.MaxDownloadKBps = 0, 0
	a.MaxUploadKBps, b.MaxUploadKBps = 0, 0
	a.MaxActiveTorrents, b.MaxActiveTorrents = 0, 0
	return a == b
}
