package service

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewInstaller,
	NewUninstaller,
	NewMounter,
)
