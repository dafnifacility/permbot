package permbot

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/pkg/k8s"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
)

// RunMain is called by the main package in cmd/permbot and is basically just a replacement for main()
func RunMain() {
	var err error
	mode := flag.String("mode", "yaml", "Mode - either yaml or k8s")
	flagNamespace := flag.String("namespace", "default", "Namespace - for yaml mode")
	flag.Parse()
	var pc types.PermbotConfig
	if cf := flag.Arg(0); cf != "" {
		err = DecodeFromFile(cf, &pc)
	} else {
		log.Fatal("specify permbot config file on commandline")
	}
	if err != nil {
		log.WithError(err).Fatal("unable to parse")
	}
	fmt.Printf("%+v\n", pc)
	switch *mode {
	case "k8s":
		cl, err := getK8SClient()
		if err != nil {
			log.WithError(err).Fatal("unable to create k8s client")
		}
		nsc := cl.CoreV1().Namespaces()
		for pcpi := range pc.Projects {
			pp := pc.Projects[pcpi]
			_, err := nsc.Get(pp.Namespace, v1.GetOptions{})
			if err != nil {
				log.WithField("namespace", pp.Namespace).WithError(err).Error("problem with namespace - doesn't exist?")
				continue
			}
			// namespace exists - create the resources
			rl, rb, err := k8s.CreateResourcesForNamespace(&pc, pp.Namespace)
			if err != nil {
				log.WithError(err).Error("unable to define resources for namespace")
			}
			rbc := cl.RbacV1()
			for _, rlr := range rl {
				newrole, err := rbc.Roles(pp.Namespace).Update(&rlr)
				if err != nil {
					log.WithError(err).Error("unable to update role")
				} else {
					log.WithFields(log.Fields{
						"role":      newrole.ObjectMeta.Name,
						"namespace": pp.Namespace,
					}).Info("created/updated role")
				}
			}
			for _, rblr := range rb {
				newrb, err := rbc.RoleBindings(pp.Namespace).Update(&rblr)
				if err != nil {
					log.WithError(err).Error("unable to update rolebinding")
				} else {
					log.WithFields(log.Fields{
						"rolebinding": newrb.ObjectMeta.Name,
						"namespace":   pp.Namespace,
					}).Info("created/updated rolebinding")
				}
			}
		}
	case "yaml":
		rres, rbres, err := k8s.CreateResourcesForNamespace(&pc, *flagNamespace)
		if err != nil {
			if os.IsNotExist(err) {
				log.WithField("namespace", *flagNamespace).Fatal("the config file doesn't have roles for the specified namespace")
			} else {
				log.WithError(err).Fatal("Failed to create resources")
			}
		}
		dumpToYaml(rres, rbres)
	default:
		log.Fatal("Unknown mode - use k8s or yaml")
	}

}

func getK8SClient() (*kubernetes.Clientset, error) {
	if kc, isset := os.LookupEnv("KUBECONFIG"); isset {
		config, err := clientcmd.BuildConfigFromFlags("", kc)
		if err != nil {
			return nil, err
		}
		return kubernetes.NewForConfig(config)
	} else {
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

func dumpToYaml(rres []rbacv1.Role, rbres []rbacv1.RoleBinding) {
	scheme := runtime.NewScheme()
	sobj := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, json.SerializerOptions{true, false, false})
	first := true
	for ri := range rres {
		if !first {
			fmt.Println("---")
		} else {
			first = false
		}
		sobj.Encode(&rres[ri], os.Stdout)
	}
	for rbi := range rbres {
		if !first {
			fmt.Println("---")
		} else {
			first = false
		}
		sobj.Encode(&rbres[rbi], os.Stdout)
	}
	// fmt.Printf("roles:\n%+v\n\nrolebindings:\n%+v\n", rres, rbres)
}

// DecodeFromFile decodes a file from `fn` into the PermbotConfig pointer `into`
func DecodeFromFile(fn string, into *types.PermbotConfig) error {
	f, err := os.Open(fn)
	if err != nil {
		return errors.Wrap(err, "unable to open config")
	}
	_, err = toml.DecodeReader(f, into)
	if err != nil {
		return errors.Wrap(err, "unable to decode config")
	}
	return err
}
