package application

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"

	"github.com/lvrach/kprof/k8scli"
	"github.com/phayes/freeport"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewApp() *cli.App {
	app := &cli.App{
		Name:  "kprof",
		Usage: "kprof: pprof go binaries running in kubernetes cluster",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "namespace",
				Value:   "",
				Aliases: []string{"n"},
				EnvVars: []string{"NAMESPACE"},
				Usage:   "namespace to use",
			},
			&cli.StringFlag{
				Name:    "container",
				Value:   "",
				Aliases: []string{"c"},
				Usage:   "container to use",
			},
			&cli.IntFlag{
				Name:    "port",
				Value:   0,
				Aliases: []string{"p"},
				Usage:   "port the pprof is listening on",
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "cpu",
				Usage:  "capture and explore CPU profile",
				Action: k8scli.WithK8S(ProfileAction),
			},
			{
				Name:   "memory",
				Usage:  "capture and explore heap profile dump",
				Action: k8scli.WithK8S(ProfileAction),
			},
			{
				Name:   "allocs",
				Usage:  "capture and explore memory allocations profile dump",
				Action: k8scli.WithK8S(ProfileAction),
			},
		},

		EnableBashCompletion: true,
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	return app
}

type PortPerContainer struct {
	Container string
	Port      int
}

func ProfileAction(c *k8scli.Context) error {
	namespace := c.Namespace()

	var profile string
	switch c.Command.Name {
	case "memory":
		profile = "heap"
	case "cpu":
		profile = "profile"
	case "allocs":
		profile = "allocs"
	default:
		return fmt.Errorf("Unknown profile type: %s", c.Command.Name)
	}

	podName := c.Args().First()

	fmt.Printf("%s profile on: %s %s \n", profile, namespace, podName)

	readyCh := make(chan struct{})
	forwardedPort, err := freeport.GetFreePort()
	if err != nil {
		return err
	}

	port := c.Int("port")
	if port == 0 {
		ports, err := detectPorts(c, podName)
		if err != nil {
			return err
		}
		if len(ports) == 0 {
			return fmt.Errorf("no ports found, you can specify one with -p / --port flag")
		}

		c := c.String("container")
		if c != "" {
			ports = lo.Filter(ports, func(x PortPerContainer, index int) bool {
				return x.Container == c
			})
		} else {
			containers := lo.Map(ports, func(item PortPerContainer, index int) string {
				return item.Container
			})
			containers = lo.Uniq(containers)
			if len(containers) > 1 {
				fmt.Println("multiple containers found:", containers, "you can specify one with -c / --container flag")
			}
		}
		port = ports[0].Port
	}

	g, ctx := errgroup.WithContext(c.Ctx())
	g.Go(func() error {
		return c.PortForward(ctx, namespace, podName, strconv.Itoa(forwardedPort), strconv.Itoa(port), readyCh)
	})
	g.Go(func() error {
		<-readyCh
		webUIPort, err := freeport.GetFreePort()
		if err != nil {
			return err
		}

		webUI := fmt.Sprintf("-http=:%d", webUIPort)
		pprofTarget := fmt.Sprintf("http://localhost:%d/debug/pprof/%s", forwardedPort, profile)

		cmd := exec.CommandContext(ctx, "go", "tool", "pprof", webUI, pprofTarget)
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr

		return cmd.Run()
	})

	return g.Wait()
}

func detectPorts(c *k8scli.Context, pod string) ([]PortPerContainer, error) {
	r, err := c.KubeClient().CoreV1().Pods(c.Namespace()).Get(c.Ctx(), pod, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var ports []PortPerContainer
	for _, container := range r.Spec.Containers {
		for _, port := range container.Ports {
			if port.Protocol != "TCP" {
				continue
			}

			ports = append(ports, PortPerContainer{
				Container: container.Name,
				Port:      int(port.ContainerPort),
			})
		}
	}

	return ports, nil
}
