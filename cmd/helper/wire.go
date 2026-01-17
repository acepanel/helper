//go:build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/acepanel/helper/internal/app"
	"github.com/acepanel/helper/internal/service"
	"github.com/acepanel/helper/internal/system"
)

func initHelper() (*app.Helper, error) {
	panic(wire.Build(
		system.ProviderSet,
		service.ProviderSet,
		app.ProviderSet,
	))
}
