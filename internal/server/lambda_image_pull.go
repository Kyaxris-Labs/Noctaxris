package server

import (
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// prepareLambdaImageRunOpts rewrites lab ECR ImageUri for DinD and attaches a lab
// registry authorization token issued as the function execution role ARN.
func (s *Server) prepareLambdaImageRunOpts(
	accountID string,
	fn store.LambdaFunction,
	env map[string]string,
	eventJSON, endpoint, eventHostPath string,
) (compute.ImageRunOpts, error) {
	pullPrincipal := strings.TrimSpace(fn.RoleARN)
	pullRef, useAuth, username, password, err := s.labRegistryPullOpts(accountID, fn.ImageURI, pullPrincipal)
	if err != nil {
		return compute.ImageRunOpts{}, err
	}
	layerPaths, err := s.store.ResolveLayerCodeDirs(s.cfg.DataRoot, accountID, fn.Layers)
	if err != nil {
		return compute.ImageRunOpts{}, err
	}
	// Lab ECR images are full-container exec; public Lambda bases use one-shot override.
	allowDefault := useAuth
	return compute.ImageRunOpts{
		ImageURI:               pullRef,
		Handler:                fn.Handler,
		TimeoutSec:             clampLambdaTimeout(fn.Timeout),
		MemoryMB:               clampLambdaMemory(fn.Memory),
		Env:                    env,
		EventJSON:              eventJSON,
		EndpointURL:            endpoint,
		EventHostPath:          eventHostPath,
		LayerHostPaths:         layerPaths,
		ListenAddr:             s.cfg.ListenAddr,
		LabRegistryPull:        useAuth,
		RegistryUsername:       username,
		RegistryPassword:       password,
		AllowDefaultEntrypoint: allowDefault,
	}, nil
}
