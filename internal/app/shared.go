package app

import (
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// DecodeFromFile decodes a file from `fn` into the PermbotConfig pointer `into`
func DecodeFromFile(fn string, into *types.PermbotConfig) error {
	f, err := os.Open(fn)
	if err != nil {
		return errors.Wrap(err, "unable to open config")
	}
	_, err = DecodeFromReader(f, into)
	return err
}

func DecodeFromReader(c io.Reader, into *types.PermbotConfig) (toml.MetaData, error) {
	return toml.DecodeReader(c, into)
}

func GetK8SClient() (*kubernetes.Clientset, error) {
	if kc, isset := os.LookupEnv("KUBECONFIG"); isset {
		config, err := clientcmd.BuildConfigFromFlags("", kc)
		if err != nil {
			return nil, err
		}
		return kubernetes.NewForConfig(config)
	}
	if home := homeDir(); home != "" {
		kcp := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kcp); err == nil {
			// File exists
			config, err := clientcmd.BuildConfigFromFlags("", kcp)
			if err != nil {
				return nil, err
			}
			return kubernetes.NewForConfig(config)
		}
	}
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
