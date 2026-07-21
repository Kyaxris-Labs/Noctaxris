package server

import (
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const lambdaRegistryPrincipal = "lambda.amazonaws.com"

// prepareLambdaImageRunOpts rewrites lab ECR ImageUri for DinD and attaches a lab
// registry authorization token (same path ECS RunTask uses for 127.0.0.1:4566 refs).
func (s *Server) prepareLambdaImageRunOpts(
	accountID string,
	fn store.LambdaFunction,
	env map[string]string,
	eventJSON, endpoint, eventHostPath string,
) (compute.ImageRunOpts, error) {
	pullRef, useAuth, username, password, err := compute.IssueLabRegistryPull(
		s.store, s.cfg.ListenAddr, accountID, fn.ImageURI, lambdaRegistryPrincipal,
	)
	if err != nil {
		return compute.ImageRunOpts{}, err
	}
	layerPaths, err := s.store.ResolveLayerCodeDirs(s.cfg.DataRoot, accountID, fn.Layers)
	if err != nil {
		return compute.ImageRunOpts{}, err
	}
	return compute.ImageRunOpts{
		ImageURI:         pullRef,
		Handler:          fn.Handler,
		TimeoutSec:       clampLambdaTimeout(fn.Timeout),
		MemoryMB:         clampLambdaMemory(fn.Memory),
		Env:              env,
		EventJSON:        eventJSON,
		EndpointURL:      endpoint,
		EventHostPath:    eventHostPath,
		LayerHostPaths:   layerPaths,
		LabRegistryPull:  useAuth,
		RegistryUsername: username,
		RegistryPassword: password,
	}, nil
}
