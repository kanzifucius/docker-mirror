// +build !darwin

package main

import (
	"encoding/base64"
	"fmt"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	log "github.com/sirupsen/logrus"
)

func getDockerCredentials(registry string) (*docker.AuthConfiguration, error) {
	authOptions, err := docker.NewAuthConfigurationsFromDockerCfg()
	if err != nil {
		log.Fatal(err)
	}

	creds, ok := authOptions.Configs[registry]
	if !ok {
		return nil, fmt.Errorf("No auth found for %s", registry)
	}

	return &creds, nil
}

func getDockerCredentialsFromAuthToken(token string) (*docker.AuthConfiguration, error) {
	log.Info("Decoding token...")
	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("Invalid token: %v", err)
	}

	parts := strings.SplitN(string(decodedToken), ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("Invalid token: expected two parts, got %d", len(parts))
	}

	log.Info("Token successfully decoded")

	return &docker.AuthConfiguration{
		Username: parts[0],
		Password: parts[1],
	}, nil
}
