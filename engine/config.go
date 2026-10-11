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
}

// sameClient reports whether a and b differ only in settings the engine
// applies itself, so switching between them needs no client restart.
func sameClient(a, b Config) bool {
	a.MinimumFreeSpaceGB, b.MinimumFreeSpaceGB = 0, 0
	a.DeleteDownloadsAfterDays, b.DeleteDownloadsAfterDays = 0, 0
	return a == b
}
