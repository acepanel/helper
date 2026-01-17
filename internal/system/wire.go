package system

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewExecutor,
	NewDetector,
	NewFirewall,
	NewSystemd,
	NewUserManager,
)
