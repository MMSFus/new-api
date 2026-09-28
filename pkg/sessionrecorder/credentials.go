package sessionrecorder

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type CredentialState string

const (
	CredentialMissing    CredentialState = "missing"
	CredentialInvalid    CredentialState = "invalid"
	CredentialConfigured CredentialState = "configured"
)

const (
	credentialAccessKeyFile = "aws_access_key_id"
	credentialSecretKeyFile = "aws_secret_access_key"
)

// Credentials holds the R2 access key pair. It is never serialised: the
// fields are excluded from JSON and String() is redacted, so it cannot leak
// into options, logs or API responses by accident.
type Credentials struct {
	AccessKeyID     string `json:"-"`
	SecretAccessKey string `json:"-"`
}

func (c Credentials) String() string   { return "Credentials(" + string(c.State()) + ")" }
func (c Credentials) GoString() string { return c.String() }

// State follows the standalone credentialPair rules.
func (c Credentials) State() CredentialState {
	access := strings.TrimSpace(c.AccessKeyID)
	secret := strings.TrimSpace(c.SecretAccessKey)
	switch {
	case access == "" && secret == "":
		return CredentialMissing
	case access == "" || secret == "":
		return CredentialInvalid
	default:
		return CredentialConfigured
	}
}

// LoadCredentials reads aws_access_key_id / aws_secret_access_key from dir
// (or $CREDENTIALS_DIRECTORY when dir is empty). When neither file exists it
// falls back to AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY.
func LoadCredentials(dir string) (Credentials, error) {
	if strings.TrimSpace(dir) == "" {
		dir = os.Getenv("CREDENTIALS_DIRECTORY")
	}
	if dir != "" {
		access, errA := readCredentialFile(filepath.Join(dir, credentialAccessKeyFile))
		secret, errS := readCredentialFile(filepath.Join(dir, credentialSecretKeyFile))
		missingA := errors.Is(errA, fs.ErrNotExist)
		missingS := errors.Is(errS, fs.ErrNotExist)
		if errA != nil && !missingA {
			return Credentials{}, errors.New("credentials: cannot read access key file")
		}
		if errS != nil && !missingS {
			return Credentials{}, errors.New("credentials: cannot read secret key file")
		}
		if !missingA || !missingS {
			return Credentials{AccessKeyID: access, SecretAccessKey: secret}, nil
		}
	}
	return Credentials{
		AccessKeyID:     strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")),
		SecretAccessKey: strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY")),
	}, nil
}

func readCredentialFile(name string) (string, error) {
	raw, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}
