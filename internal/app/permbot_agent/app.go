package permbot_agent

import (
	"context"
	"embed"
	"fmt"
	htemplate "html/template"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/urfave/cli/v2"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/app"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/pkg/k8s"
	"gitlab.dafni.rl.ac.uk/dafni/tools/permbot/pkg/types"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const (
	configMapKey          = "permbot.toml"
	flagOwnerRefName      = "owner"
	flagSlackWebhookURL   = "slack-webhook"
	flagJSONLogs          = "json-logs"
	flagDebug             = "debug"
	flagHTTPListenAddress = "http-listen"
)

var (
	flagConfigMap = &cli.StringFlag{
		Name:    "configmap",
		Aliases: []string{"f"},
		EnvVars: []string{"PERMBOT_CONFIGMAP"},
		Value:   "permbot/config",
	}
)

//go:embed template/*
var templates embed.FS

type PermbotAgent struct {
	kclient         *kubernetes.Clientset
	configMapName   v1.ObjectMeta
	ownerRef        string
	slackWebhookURL string

	metrics struct {
		changeCount *prometheus.CounterVec
		applyTime   prometheus.Histogram
	}
}

func CreateAgent(cc *cli.Context) (*PermbotAgent, error) {
	cmmd := configMapName(cc.String(flagConfigMap.Name))
	cl, err := app.GetK8SClient()
	if err != nil {
		return nil, err
	}

	pa := &PermbotAgent{
		kclient:         cl,
		configMapName:   cmmd,
		ownerRef:        cc.String(flagOwnerRefName),
		slackWebhookURL: cc.String(flagSlackWebhookURL),
	}
	pa.setupMetrics()
	pa.setupHTTP(cc.String(flagHTTPListenAddress))
	return pa, nil
}

func (pa *PermbotAgent) setupMetrics() {
	pa.metrics.changeCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "permbot",
		Subsystem: "agent",
		Name:      "changes_applied",
		Help:      "Count of times the rules have been applied (success/error)",
	}, []string{"outcome"})
	pa.metrics.applyTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "permbot",
		Subsystem: "agent",
		Name:      "applytime_secs",
		Help:      "Time taken in seconds to apply the rules to Kubernetes following their conversion to C/R/Bs",
	})
}

func httpLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.WithFields(log.Fields{
			"remote-host": r.RemoteAddr,
		}).Infof("%s %s", r.Method, r.URL.Path)
		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}

func (pa *PermbotAgent) setupHTTP(listenAddr string) {
	mux := mux.NewRouter()
	mux.Use(httpLogger)
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			rw.WriteHeader(http.StatusNotFound)
		}
		tmpl := htemplate.Must(htemplate.ParseFS(templates, "template/html/index.html"))
		type innerMsg struct {
			Owner   string
			Version string
		}
		tmpl.Execute(rw, innerMsg{Owner: pa.ownerRef, Version: app.Version()})
	}).Methods(http.MethodGet)
	mux.HandleFunc("/healthz", func(rw http.ResponseWriter, req *http.Request) { rw.WriteHeader(200) }).Methods(http.MethodGet)
	mux.Handle("/metrics", promhttp.Handler())
	if listenAddr != "" {
		go http.ListenAndServe(listenAddr, mux)
	}
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

type slackStatus struct {
	Success bool
}

func (pa *PermbotAgent) notifySlack(success bool) {
	if pa.slackWebhookURL == "" {
		log.Debug("skipping slack notification because: webhook URL not defined")
		return
	}
	st := template.Must(template.New("slack").ParseFS(templates, "template/*.txt"))
	log.WithField("tmpl", st.DefinedTemplates()).Debug("loaded templates")
	msgbuf := new(strings.Builder)
	st.ExecuteTemplate(msgbuf, "slack.txt", slackStatus{Success: success})
	msg := &slack.WebhookMessage{
		Username: "Permbot",
		Text:     msgbuf.String(),
	}
	err := slack.PostWebhook(pa.slackWebhookURL, msg)
	if err != nil {
		log.WithError(err).Error("unable to post Slack webhook, but I can't really do anything about that")
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
	tStart := time.Now()
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
	tDuration := time.Since(tStart)
	pa.metrics.applyTime.Observe(float64(tDuration.Seconds()))
	pa.notifySlack(true)
	return nil
}

func (pa *PermbotAgent) processConfig(ctx context.Context, ev watch.EventType) error {
	switch ev {
	case watch.Deleted:
		return fmt.Errorf("configmap received deletion event")
	default:
		// added, modified, ??
		err := pa.processChangedConfig(ctx, false)
		if err == nil {
			pa.metrics.changeCount.WithLabelValues("success").Inc()
		} else {
			pa.metrics.changeCount.WithLabelValues("error").Inc()
		}
		return err
	}
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
					&cli.StringFlag{
						Name:    flagOwnerRefName,
						Usage:   "Owner ref to add to created labels",
						EnvVars: []string{"PERMBOT_OWNER"},
					},
					&cli.StringFlag{
						Name:    flagSlackWebhookURL,
						Usage:   "Slack webhook to trigger on config change",
						EnvVars: []string{"SLACK_WEBHOOK"},
					},
					&cli.StringFlag{
						Name:    flagHTTPListenAddress,
						Usage:   "HTTP Listen address (for OpenMetrics)",
						EnvVars: []string{"HTTP_LISTEN"},
					},
				},
				Action: runAgent,
			},
		},
		Flags: []cli.Flag{
			cli.VersionFlag,
			&cli.BoolFlag{
				Name:    flagDebug,
				Aliases: []string{"d"},
				Usage:   "Enable debug logs",
				EnvVars: []string{"DEBUG"},
			},
			&cli.BoolFlag{
				Name:    flagJSONLogs,
				Usage:   "Enable JSON log format",
				EnvVars: []string{"JSON_LOGS"},
			},
		},
		Before: func(cc *cli.Context) error {
			if cc.Bool(flagDebug) {
				log.SetLevel(log.DebugLevel)
				log.SetReportCaller(true)
			}
			if cc.Bool(flagJSONLogs) {
				log.SetFormatter(&log.JSONFormatter{})
			}
			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.WithError(err).Fatal("fatal error")
	}
}
