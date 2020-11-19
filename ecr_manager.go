package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"
)

type ecrManager struct {
	client       *ecr.Client     // AWS ECR client
	repositories map[string]bool // list of repositories in ECR
}

func (e *ecrManager) exists(name string) bool {
	if _, ok := e.repositories[name]; ok {
		return true
	}

	return false
}

func (e *ecrManager) ensure(name string) error {
	if e.exists(name) {
		return nil
	}

	return e.create(name)
}

func (e *ecrManager) create(name string) error {
	_, err := e.client.CreateRepository(context.Background(), &ecr.CreateRepositoryInput{
		RepositoryName: &name,
	})
	if err != nil {
		return err
	}

	e.repositories[name] = true
	return nil
}

func (e *ecrManager) Login() (string, error) {
	log.Info("Obtaining authorization token from AWS...")
	output, err := e.client.GetAuthorizationToken(context.Background(), &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		log.Errorf("Unable to obtain authorization token: %s", err)
	}

	var auth string

	for _, authData := range output.AuthorizationData {
		auth = *authData.AuthorizationToken
	}

	log.Info("Authorization token obtained successfully from AWS...")

	return auth, err
}

func (e *ecrManager) buildCache(nextToken *string) error {
	if nextToken == nil {
		log.Info("Loading list of ECR repositories")
	}

	resp, err := e.client.DescribeRepositories(context.Background(), &ecr.DescribeRepositoriesInput{
		NextToken: nextToken,
	})
	if err != nil {
		return err
	}

	if e.repositories == nil {
		e.repositories = make(map[string]bool)
	}

	for _, repo := range resp.Repositories {
		e.repositories[*repo.RepositoryName] = true
	}

	// keep paging as long as there is a token for the next page
	if resp.NextToken != nil {
		e.buildCache(resp.NextToken)
	}

	// no next token means we hit the last page
	if nextToken == nil {
		log.Info("Done loading ECR repositories")
	}

	return nil
}

func (e *ecrManager) buildCacheBackoff() backoff.Operation {
	return func() error {
		return e.buildCache(nil)
	}
}
