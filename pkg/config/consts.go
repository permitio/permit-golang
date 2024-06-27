package config

import "time"

// Default Builder consts
const (
	DefaultApiUrl    = "https://api.permit.io"
	DefaultDebugMode = false
	DefaultPdpUrl    = "http://localhost:7766"
	DefaultOpaUrl    = "http://localhost:8181"

	DefaultTimeout          = time.Second * 5
	DefaultFactsSyncTimeout = 10 * time.Second
)
