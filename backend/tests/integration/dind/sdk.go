package dind

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"strings"

	"github.com/cpuguy83/dockercfg"
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

type credentialResolver func(string) (registry.AuthConfig, error)

// AuthenticatedClient forwards the developer's existing CLI login only to the
// registry being pulled, in integration fixtures. Embedding the SDK preserves
// ContainerLogs, inspection and Close for the real browser test server.
type AuthenticatedClient struct {
	*client.Client
	resolve credentialResolver
}

// NewSDKClient requires an explicit private DinD endpoint; it never falls back
// to the host daemon. Credentials remain in process and are never saved.
func NewSDKClient(host string) (*AuthenticatedClient, error) {
	if host == "" {
		return nil, errors.New("integration SDK requires an explicit private Docker host")
	}
	api, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &AuthenticatedClient{Client: api, resolve: cliCredentials}, nil
}

func cliCredentials(server string) (registry.AuthConfig, error) {
	// dockercfg is already used by testcontainers. Its config and helper APIs
	// honor DOCKER_CONFIG, per-registry helpers, the default store and tokens.
	cfg, err := dockercfg.LoadDefaultConfig()
	if errors.Is(err, fs.ErrNotExist) {
		return registry.AuthConfig{}, nil
	}
	if err != nil {
		return registry.AuthConfig{}, errors.New("integration Docker CLI configuration unavailable")
	}
	user, secret, err := cfg.GetRegistryCredentials(server)
	if err != nil {
		// Helper errors can contain credentials on stdout/stderr. Never wrap them.
		return registry.AuthConfig{}, errors.New("integration Docker credential lookup failed")
	}
	auth := registry.AuthConfig{ServerAddress: server, Username: user}
	if user == "" {
		auth.IdentityToken = secret
	} else {
		auth.Password = secret
	}
	return auth, nil
}

func (c *AuthenticatedClient) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	if opts.RegistryAuth == "" && opts.PrivilegeFunc == nil {
		named, err := reference.ParseNormalizedNamed(ref)
		if err != nil {
			return nil, errors.New("invalid integration pull reference")
		}
		server := reference.Domain(named)
		host := server
		if h, _, err := net.SplitHostPort(server); err == nil {
			host = h
		}
		ip := net.ParseIP(strings.Trim(host, "[]"))
		// Local fixture registries are anonymous and must not invoke user helpers.
		if host != "localhost" && !(ip != nil && ip.IsLoopback()) {
			server = dockercfg.ResolveRegistryHost(server)
			auth, err := c.resolve(server)
			if err != nil {
				return nil, errors.New("integration Docker credential lookup failed")
			}
			if auth.Username != "" || auth.Password != "" || auth.IdentityToken != "" || auth.RegistryToken != "" {
				opts.RegistryAuth, err = registry.EncodeAuthConfig(auth)
				if err != nil {
					return nil, errors.New("integration Docker authentication encoding failed")
				}
			}
		}
	}
	return c.Client.ImagePull(ctx, ref, opts)
}
