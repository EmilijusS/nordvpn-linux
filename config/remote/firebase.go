package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/coreos/go-semver/semver"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/firebaseremoteconfig/v1"
	"google.golang.org/api/option"
)

const (
	firebaseTimeout   = time.Second * 8
	minimalVersionKey = "min_version"
	remoteConfigScope = "https://www.googleapis.com/auth/firebase.remoteconfig"
)

type ServiceAccount struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

type RConfig struct {
	updatePeriod time.Duration
	lastUpdate   time.Time
	config       *firebaseremoteconfig.RemoteConfig
	serviceToken string
}

// NewRConfig creates instance of remote config
func NewRConfig(updatePeriod time.Duration, serviceToken string) *RConfig {
	return &RConfig{
		updatePeriod: updatePeriod,
		serviceToken: serviceToken,
	}
}

func (rc *RConfig) fetchRemoteConfig() error {
	var serviceAccount ServiceAccount
	err := json.Unmarshal([]byte(rc.serviceToken), &serviceAccount)
	if err != nil {
		return fmt.Errorf("could not load service account data, %w", err)
	}

	// Add context timeout for no net situation
	timeoutCtx, cancel := context.WithTimeout(context.Background(), firebaseTimeout)
	timeoutCtx = context.WithValue(timeoutCtx, oauth2.HTTPClient, &http.Client{Timeout: firebaseTimeout})
	defer cancel()

	creds, err := google.CredentialsFromJSON(timeoutCtx, []byte(rc.serviceToken), remoteConfigScope)
	if err != nil {
		return fmt.Errorf("could not load credentials, %w", err)
	}
	firebaseremoteconfigService, err := firebaseremoteconfig.NewService(timeoutCtx, option.WithCredentials(creds))
	if err != nil {
		return fmt.Errorf("could not create new firebase service, %w", err)
	}

	// Get project remote config
	fireBaseRemoteConfigCall := firebaseremoteconfigService.Projects.GetRemoteConfig("projects/" + serviceAccount.ProjectID).Context(timeoutCtx)

	remoteConfig, err := fireBaseRemoteConfigCall.Do()
	if err != nil || remoteConfig == nil {
		return fmt.Errorf("could not execute firebase remote config call, %w", err)
	}
	if remoteConfig.ServerResponse.HTTPStatusCode != http.StatusOK {
		return fmt.Errorf("invalid remote config server HTTP response: %d", remoteConfig.ServerResponse.HTTPStatusCode)
	}

	rc.config = remoteConfig
	return nil
}

func (rc *RConfig) fetchRemoteConfigIfTime() error {
	if time.Now().After(rc.lastUpdate.Add(rc.updatePeriod)) {
		rc.lastUpdate = time.Now()
		err := rc.fetchRemoteConfig()
		if err != nil {
			return err
		}
	}

	if rc.config == nil {
		return fmt.Errorf("no remote config value")
	}

	return nil
}

// GetValue provides value of requested key from remote config
func (rc *RConfig) GetValue(cfgKey string) (string, error) {
	if err := rc.fetchRemoteConfigIfTime(); err != nil {
		return "", err
	}

	configParam := rc.config.Parameters
	for key, val := range configParam {
		if key == cfgKey {
			return val.DefaultValue.Value, nil
		}
	}
	return "", fmt.Errorf("key %s does not exist in remote config", cfgKey)
}

func stringToSemVersion(stringVersion, prefix string) (*semver.Version, error) {
	// removing test suffix if any
	stringVersion = strings.TrimSuffix(stringVersion, "_test")
	// removing predefined prefix
	stringVersion = strings.TrimPrefix(stringVersion, prefix)
	// if version development, remove extra suffix
	if strings.Contains(stringVersion, "+") {
		stringVersion = strings.Split(stringVersion, "+")[0]
	}
	// if version has v added, remove it.
	stringVersion = strings.Replace(stringVersion, "v", "", 1)
	// in remote config field name dots are not allowed, using underscores instead,
	// need to replace here
	stringVersion = strings.ReplaceAll(stringVersion, "_", ".")
	return semver.NewVersion(stringVersion)
}

// GetTelioConfig try to find remote config field for app version
// and load json block from that field
func (rc *RConfig) GetTelioConfig(stringVersion string) (string, error) {
	cfg := ""

	if err := rc.fetchRemoteConfigIfTime(); err != nil {
		return cfg, err
	}

	appVersion, err := stringToSemVersion(stringVersion, "")
	if err != nil {
		return cfg, err
	}

	// build descending ordered version list
	orderedFields := []*fieldVersion{}
	for key := range rc.config.Parameters {
		if strings.HasPrefix(key, RcTelioConfigFieldPrefix) {
			ver, err := stringToSemVersion(key, RcTelioConfigFieldPrefix)
			if err != nil {
				log.Println(err)
				continue
			}
			orderedFields = insertFieldVersion(orderedFields, &fieldVersion{ver, key})
		}
	}

	// find exact version match or first older/lower version
	versionField, err := findVersionField(orderedFields, appVersion)
	if err != nil {
		return cfg, err
	}
	log.Println("remote config version field:", versionField)

	jsonString, err := rc.GetValue(versionField)
	if err != nil {
		return cfg, err
	}

	return jsonString, nil
}

type fieldVersion struct {
	version   *semver.Version
	fieldName string
}

func insertFieldVersion(sourceArray []*fieldVersion, s *fieldVersion) []*fieldVersion {
	// build list descending order by sem version
	i := sort.Search(len(sourceArray), func(i int) bool { return sourceArray[i].version.Compare(*s.version) <= 0 })
	sourceArray = append(sourceArray, nil)
	copy(sourceArray[i+1:], sourceArray[i:])
	sourceArray[i] = s
	return sourceArray
}

func findVersionField(sourceArray []*fieldVersion, appVersion *semver.Version) (string, error) {
	for _, item := range sourceArray {
		// looking for exact or older version
		if appVersion.Compare(*item.version) >= 0 {
			return item.fieldName, nil
		}
	}
	return "", errors.New("version field not found in remote config")
}
