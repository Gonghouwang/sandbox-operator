package controller

import "sandbox-operator/internal/openapi"

func sandboxOpenAPIDebugValues(remote openapi.Sandbox) []any {
	return []any{
		"remoteHasUrls", remote.URLs != nil,
		"remoteUrlsFieldCount", sandboxURLFieldCount(remote.URLs),
		"remoteHasAccessUrl", remote.AccessURL != nil,
		"remoteAccessUrlFieldCount", sandboxURLFieldCount(remote.AccessURL),
		"remoteSdnsUrlCount", len(remote.SdnsURLs),
		"remoteHasCustomConfiguration", remote.CustomConfiguration != nil,
		"remoteEnvCount", len(remote.Envs),
		"remoteHasKS3MountConfig", remote.KS3MountConfig != nil,
		"remoteHasKPFSMountConfig", remote.KPFSMountConfig != nil,
	}
}

func sandboxURLFieldCount(urls *openapi.URLs) int {
	if urls == nil {
		return 0
	}
	count := 0
	if urls.CdpURL != "" {
		count++
	}
	if urls.NoVncURL != "" {
		count++
	}
	if urls.Code != "" {
		count++
	}
	if urls.AppURL != "" {
		count++
	}
	if urls.TerminalURL != "" {
		count++
	}
	if urls.VscodeURL != "" {
		count++
	}
	return count
}
