package permbot

import (
	"context"
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/app"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/pkg/k8s"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
)

// RunMain is called by the main package in cmd/permbot and is basically just a replacement for main()
func RunMain() {
	ctx := context.Background()
	var err error
	mode := flag.String("mode", "yaml", "Mode - either yaml or k8s")
	flagNamespace := flag.String("namespace", "", "Only dump specific namespace - for yaml mode")
	flagGlobal := flag.Bool("global", true, "Also create/display globally scoped resources (ClusterRole/ClusterRoleBinding)")
	flagDebug := flag.Bool("debug", false, "Enable debug logging")
	flagOwner := flag.String("owner", "permbot", "Owner value for Kubernetes label")
	flagRulesRef := flag.String("ref", "", fmt.Sprintf("Version of input repository to include in rule annotations (%s)", k8s.AnnotationRulesRef))
	flagVersion := flag.Bool("version", false, "Exit, only printing Permbot version")
	flag.Parse()
	if *flagDebug {
		log.SetLevel(log.DebugLevel)
	}
	log.WithField("version", app.Version()).Info("Permbot")
	if *flagVersion {
		return
	}
	var pc types.PermbotConfig
	if cf := flag.Arg(0); cf != "" {
		err = app.DecodeFromFile(cf, &pc)
	} else {
		log.Fatal("specify permbot config file on commandline")
	}
	if err != nil {
		log.WithError(err).Fatal("unable to parse")
	}
	// fmt.Printf("%+v\n", pc)
	switch *mode {
	case "k8s":
		cl, err := app.GetK8SClient()
		if err != nil {
			log.WithError(err).Fatal("unable to create k8s client")
		}
		rbc := cl.RbacV1()
		nsc := cl.CoreV1().Namespaces()
		for pcpi := range pc.Projects {
			pp := pc.Projects[pcpi]
			_, err := nsc.Get(ctx, pp.Namespace, v1.GetOptions{})
			if err != nil {
				log.WithField("namespace", pp.Namespace).WithError(err).Error("problem with namespace - doesn't exist?")
				continue
			}
			// namespace exists - create the resources
			rl, rb, err := k8s.CreateResourcesForNamespace(&pc, pp.Namespace, *flagRulesRef, *flagOwner)
			if err != nil {
				log.WithError(err).Error("unable to define resources for namespace")
			}
			for _, rlr := range rl {
				newrole, err := rbc.Roles(pp.Namespace).Update(ctx, &rlr, v1.UpdateOptions{})
				if err != nil {
					log.WithError(err).WithField("project", &rlr.Name).Error("unable to update role")
				} else {
					log.WithFields(log.Fields{
						"role":      newrole.ObjectMeta.Name,
						"namespace": pp.Namespace,
					}).Info("created/updated role")
				}
			}
			for _, rblr := range rb {
				newrb, err := rbc.RoleBindings(pp.Namespace).Update(ctx, &rblr, v1.UpdateOptions{})
				if err != nil {
					log.WithError(err).WithField("project", &rblr.Name).Error("unable to update rolebinding")
				} else {
					log.WithFields(log.Fields{
						"rolebinding": newrb.ObjectMeta.Name,
						"namespace":   pp.Namespace,
					}).Info("created/updated rolebinding")
				}
			}
		}
		if *flagGlobal {
			// Done with the namespace-scoped resources, next up is the Global ones
			crl, crb, err := k8s.CreateGlobalResources(&pc, *flagRulesRef, *flagOwner)
			if err != nil {
				log.WithError(err).Fatal("unable to create globally scoped resources")
			}
			for crli := range crl {
				newcr, err := rbc.ClusterRoles().Update(ctx, &crl[crli], v1.UpdateOptions{})
				if err != nil {
					log.WithError(err).WithField("role", crl[crli].Name).Error("unable to update clusterrole")
				} else {
					log.WithField("clusterrole", newcr.Name).Info("created/updated clusterrole")
				}
			}
			for crlbi := range crb {
				newcrb, err := rbc.ClusterRoleBindings().Update(ctx, &crb[crlbi], v1.UpdateOptions{})
				if err != nil {
					log.WithError(err).WithField("role", &crb[crlbi].Name)
				} else {
					log.WithField("clusterrolebinding", newcrb.Name).Info("created/updated clusterrolebinding")
				}
			}
		}
	case "yaml":
		if *flagNamespace != "" {
			log.WithField("namespace", *flagNamespace).Debug("dumping single namespace")
			dumpYAMLNamespace(&pc, *flagNamespace, *flagRulesRef, *flagOwner)
		} else {
			log.Debug("no namespace specified - dumping all")
			for _, nns := range pc.Projects {
				dumpYAMLNamespace(&pc, nns.Namespace, *flagRulesRef, *flagOwner)
				fmt.Println("--")
			}
		}
		if *flagGlobal {
			fmt.Println("--")
			crres, crbres, err := k8s.CreateGlobalResources(&pc, *flagRulesRef, *flagOwner)
			if err != nil {
				log.WithError(err).Fatal("Failed to create global resources")
			}
			dumpGlobalToYaml(crres, crbres)
		}
	default:
		log.Fatal("Unknown mode - use k8s or yaml")
	}
}

func dumpYAMLNamespace(pc *types.PermbotConfig, ns, rulesRef, owner string) {
	rres, rbres, err := k8s.CreateResourcesForNamespace(pc, ns, rulesRef, owner)
	if err != nil {
		if os.IsNotExist(err) {
			log.WithField("namespace", ns).Warn("the config file doesn't have roles for the specified namespace")
		} else {
			log.WithError(err).Fatal("Failed to create resources")
		}
	}
	dumpToYaml(rres, rbres)
}

func dumpGlobalToYaml(crres []rbacv1.ClusterRole, crbres []rbacv1.ClusterRoleBinding) {
	scheme := runtime.NewScheme()
	sobj := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, json.SerializerOptions{Yaml: true, Pretty: false, Strict: false})
	first := true
	for ri := range crres {
		if !first {
			fmt.Println("---")
		} else {
			first = false
		}
		sobj.Encode(&crres[ri], os.Stdout)
	}
	for rbi := range crbres {
		if !first {
			fmt.Println("---")
		} else {
			first = false
		}
		sobj.Encode(&crbres[rbi], os.Stdout)
	}
}

func dumpToYaml(rres []rbacv1.Role, rbres []rbacv1.RoleBinding) {
	scheme := runtime.NewScheme()
	sobj := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, json.SerializerOptions{Yaml: true, Pretty: false, Strict: false})
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
