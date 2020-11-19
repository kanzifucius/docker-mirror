package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/cenkalti/backoff"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/go-co-op/gocron"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var (
	progConf Config
)

// Config is the result of the parsed yaml file
type Config struct {
	Workers         int          `yaml:"workers"`
	Repositories    []Repository `yaml:"repositories,flow"`
	Target          TargetConfig `yaml:"target"`
	Oidc            bool         `yaml:"enableOidc"`
	ScheduleMinutes uint64       `yaml:"scheduleMinutes"`
}

// TargetConfig contains info on where to mirror repositories to
type TargetConfig struct {
	Registry string `yaml:"registry"`
	Prefix   string `yaml:"prefix"`
}

// Repository is a single docker hub repository to mirror
type Repository struct {
	Name            string            `yaml:"name"`
	MatchTags       []string          `yaml:"match_tag"`
	DropTags        []string          `yaml:"ignore_tag"`
	MaxTags         int               `yaml:"max_tags"`
	MaxTagAge       *Duration         `yaml:"max_tag_age"`
	RemoteTagSource string            `yaml:"remote_tags_source"`
	RemoteTagConfig map[string]string `yaml:"remote_tags_config"`
	TargetPrefix    *string           `yaml:"target_prefix"`
}

func main() {
	log.Info("Reading config from file")
	// mirror file to read
	configFile := "config.yaml"
	if f := os.Getenv("CONFIG_FILE"); f != "" {
		configFile = f
	}

	content, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal(fmt.Sprintf("Could not read config file: %s", err))
	}

	if err := yaml.Unmarshal(content, &progConf); err != nil {
		log.Fatal(fmt.Sprintf("Could not parse config file: %s", err))
	}

	log.Infof("Scheduling job to run every %v minutes", progConf.ScheduleMinutes)
	sched := gocron.NewScheduler(time.UTC)
	// sched.Every(1).Day().Do(startJob)
	sched.Every(progConf.ScheduleMinutes).Minutes().Do(startJob)
	sched.StartBlocking()
}

func startJob() {
	log.Info("Starting run of scheduled mirror job")
	// log level
	if rawLevel := os.Getenv("LOG_LEVEL"); rawLevel != "" {
		logLevel, err := log.ParseLevel(rawLevel)
		if err != nil {
			log.Fatal(err)
		}
		log.SetLevel(logLevel)
	}

	if progConf.Target.Registry == "" {
		log.Fatal("Missing `target -> registry` yaml config")
	}

	if progConf.Workers == 0 {
		progConf.Workers = runtime.NumCPU()
	}

	// number of workers
	if w := os.Getenv("NUM_WORKERS"); w != "" {
		p, err := strconv.Atoi(w)
		if err != nil {
			log.Fatal(fmt.Sprintf("Could not parse NUM_WORKERS env: %s", err))
		}

		progConf.Workers = p
	}

	// init Docker client
	log.Info("Creating Docker client")
	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatalf("Could not create Docker client: %s", err.Error())
	}

	info, err := client.Info()
	if err != nil {
		log.Fatalf("Could not get Docker info: %s", err.Error())
	}
	log.Infof("Connected to Docker daemon: %s @ %s", info.Name, info.ServerVersion)

	// init AWS client
	log.Info("Creating AWS client")
	cfg, err := config.LoadDefaultConfig()
	if err != nil {
		log.Fatalf("Unable to load AWS SDK config, " + err.Error())
	}

	// pre-load ECR repositories
	ecrManager := &ecrManager{client: ecr.NewFromConfig(cfg)}

	backoffSettings := backoff.NewExponentialBackOff()
	backoffSettings.InitialInterval = 1 * time.Second
	backoffSettings.MaxElapsedTime = 10 * time.Second

	notifyError := func(err error, d time.Duration) {
		log.Errorf("%v (%s)", err, d.String())
	}

	if err = backoff.RetryNotify(ecrManager.buildCacheBackoff(), backoffSettings, notifyError); err != nil {
		log.Fatalf("Could not build ECR cache: %s", err)
	}

	workerCh := make(chan Repository, 5)
	var wg sync.WaitGroup

	// start background workers
	for i := 0; i < progConf.Workers; i++ {
		go worker(&wg, workerCh, client, ecrManager)
	}

	prefix := os.Getenv("PREFIX")

	// add jobs for the workers
	for _, repo := range progConf.Repositories {
		if prefix != "" && !strings.HasPrefix(repo.Name, prefix) {
			continue
		}

		wg.Add(1)
		workerCh <- repo
	}

	// wait for all workers to complete
	wg.Wait()
	log.Info("Done")
}

func worker(wg *sync.WaitGroup, workerCh chan Repository, dc *docker.Client, ecrm *ecrManager) {
	log.Debug("Starting worker")

	for {
		select {
		case repo := <-workerCh:
			m := mirror{
				dockerClient: dc,
				ecrManager:   ecrm,
			}
			if err := m.setup(repo); err != nil {
				log.Errorf("Failed to setup mirror for repository %s: %s", repo.Name, err)
				wg.Done()
				continue
			}

			m.work()
			wg.Done()
		}
	}
}
