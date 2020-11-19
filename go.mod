module seatgeek/docker-mirror

go 1.14

require (
	github.com/aws/aws-sdk-go-v2/config v0.2.2
	github.com/aws/aws-sdk-go-v2/service/ecr v0.29.0
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/docker/docker-credential-helpers v0.6.3
	github.com/fsouza/go-dockerclient v1.6.6
	github.com/go-co-op/gocron v0.3.3
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/ryanuber/go-glob v1.0.0
	github.com/sirupsen/logrus v1.7.0
	gopkg.in/yaml.v2 v2.3.0
)
