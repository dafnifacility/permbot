package permbot_agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/app"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/pkg/k8s"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const (
	configMapKey     = "permbot.toml"
	flagOwnerRefName = "owner"
)

var (
	flagConfigMap = &cli.StringFlag{
		Name:    "configmap",
		Aliases: []string{"f"},
		EnvVars: []string{"PERMBOT_CONFIGMAP"},
		Value:   "permbot/config",
	}
)

type PermbotAgent struct {
	kclient       *kubernetes.Clientset
	configMapName v1.ObjectMeta
	ownerRef      string
}

func CreateAgent(cc *cli.Context) (*PermbotAgent, error) {
	cmmd := configMapName(cc.String(flagConfigMap.Name))
	cl, err := app.GetK8SClient()
	if err != nil {
		return nil, err
	}
	return &PermbotAgent{
		kclient:       cl,
		configMapName: cmmd,
		ownerRef:      cc.String(flagOwnerRefName),
	}, nil
}

func (pa *PermbotAgent) Run(ctx context.Context) error {
	if _, _, err := pa.getConfigMap(ctx); err != nil {
		return err
	}
	wi, err := pa.kclient.CoreV1().ConfigMaps(pa.configMapName.Namespace).Watch(ctx, v1.SingleObject(pa.configMapName))
	if err != nil {
		return err
	}
	for {
		log.Debug("waiting for configmap change event...")
		ev := <-wi.ResultChan()
		log.WithField("event", ev.Type).Debug("received configmap watch event")
		pa.processConfig(ctx, ev.Type)
	}
}

func configMapName(rawName string) v1.ObjectMeta {
	p := strings.SplitN(rawName, "/", 2)
	if len(p) == 1 {
		return v1.ObjectMeta{Namespace: "default", Name: p[0]}
	}
	return v1.ObjectMeta{Namespace: p[0], Name: p[1]}
}

func (pa *PermbotAgent) getConfigMap(ctx context.Context) (*types.PermbotConfig, *v1.ObjectMeta, error) {
	cm, err := pa.kclient.CoreV1().ConfigMaps(pa.configMapName.Namespace).Get(ctx, pa.configMapName.Name, v1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	md := &cm.ObjectMeta
	kdata := cm.Data[configMapKey]
	// Check that the config entry isn't empty
	if kdata == "" {
		return nil, nil, fmt.Errorf("configmap key `%s` is empty", configMapKey)
	}
	cs := strings.NewReader(kdata)
	var cfg types.PermbotConfig
	_, err = app.DecodeFromReader(cs, &cfg)
	// TODO: do something with the metadata here - sanity checks?
	return &cfg, md, err
}

func (pa *PermbotAgent) applyGlobalResources(ctx context.Context, rulesRef string, cm *types.PermbotConfig, dryRun bool) error {
	gr, grb, err := k8s.CreateGlobalResources(cm, rulesRef, pa.ownerRef)
	if err != nil {
		return err
	}
	rc := pa.kclient.RbacV1()
	var uopts v1.UpdateOptions
	if dryRun {
		uopts.DryRun = []string{"All"}
	}
	for _, cr := range gr {
		ncr, err := rc.ClusterRoles().Update(ctx, &cr, uopts)
		if err != nil {
			return err
		}
		log.WithField("newClusterRole", ncr).Debug("new cluster role applied")
	}
	for _, crb := range grb {
		ncr, err := rc.ClusterRoleBindings().Update(ctx, &crb, uopts)
		if err != nil {
			return err
		}
		log.WithField("newClusterRoleBinding", ncr).Debug("new cluster role binding applied")
	}
	return nil
}

func (pa *PermbotAgent) applyNamespacedResources(ctx context.Context, rulesRef string, cm *types.PermbotConfig, ns string, dryRun bool) error {
	nlog := log.WithField("namespace", ns)
	nsr, nsrb, err := k8s.CreateResourcesForNamespace(cm, ns, rulesRef, pa.ownerRef)
	if err != nil {
		return err
	}
	rc := pa.kclient.RbacV1()
	var uopts v1.UpdateOptions
	if dryRun {
		uopts.DryRun = []string{"All"}
	}
	for _, r := range nsr {
		ncr, err := rc.Roles(ns).Update(ctx, &r, uopts)
		if err != nil {
			return err
		}
		nlog.WithField("newRole", ncr).Debug("new role applied")
	}
	for _, rb := range nsrb {
		ncr, err := rc.RoleBindings(ns).Update(ctx, &rb, uopts)
		if err != nil {
			return err
		}
		nlog.WithField("newRoleBinding", ncr).Debug("new role binding applied")
	}
	return nil
}

func rulesRefFromMeta(md *v1.ObjectMeta) string {
	if md == nil {
		return "unknown"
	}
	mdlabel := md.Annotations["dafni.ac.uk/permbot-rules-ref"]
	if mdlabel != "" {
		return mdlabel
	} else {
		return "unknown"
	}
}

func (pa *PermbotAgent) processChangedConfig(ctx context.Context, dryRun bool) error {
	cm, md, err := pa.getConfigMap(ctx)
	if err != nil {
		return err
	}
	log.Infof("applying changed configmap with %d projects, %d roles", len(cm.Projects), len(cm.Roles))
	// FIXME: this should be based on the configmap version/git ref (annotation?)
	rulesRef := rulesRefFromMeta(md)
	err = pa.applyGlobalResources(ctx, rulesRef, cm, dryRun)
	if err != nil {
		log.WithError(err).Error("unable to apply global roles")
	}
	for _, nsc := range cm.Projects {
		err = pa.applyNamespacedResources(ctx, rulesRef, cm, nsc.Namespace, dryRun)
		if err != nil {
			log.WithField("namespace", nsc.Namespace).WithError(err).Error("unable to apply namespaced roles")
		}
	}
	return nil
}

func (pa *PermbotAgent) processConfig(ctx context.Context, ev watch.EventType) error {
	switch ev {
	case watch.Deleted:
		return fmt.Errorf("configmap received deletion event")
	case watch.Added:
		fallthrough
	case watch.Modified:
		return pa.processChangedConfig(ctx, false)
	}
	return nil
}

func runAgent(cc *cli.Context) error {
	pa, err := CreateAgent(cc)
	if err != nil {
		return err
	}
	return pa.Run(cc.Context)
}

func RunMain() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:    "run",
				Usage:   "Run forever, watching a configmap and applying changes as required",
				Aliases: []string{"r"},
				Flags: []cli.Flag{
					flagConfigMap,
				},
				Action: runAgent,
			},
		},
		Flags: []cli.Flag{
			cli.VersionFlag,
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "Enable debug logs",
			},
			&cli.BoolFlag{
				Name:  "json-logs",
				Usage: "Enable JSON log format",
			},
			&cli.StringFlag{
				Name:    flagOwnerRefName,
				Usage:   "Owner ref to add to created labels",
				EnvVars: []string{"PERMBOT_OWNER"},
			},
		},
		Before: func(cc *cli.Context) error {
			if cc.Bool("debug") {
				log.SetLevel(log.DebugLevel)
			}
			log.SetReportCaller(true)
			if cc.Bool("json-logs") {
				log.SetFormatter(&log.JSONFormatter{})
			}
			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.WithError(err).Fatal("fatal error")
	}
}
