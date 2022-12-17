package k8scli

import (
	"context"
	"net/http"
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"

	"github.com/urfave/cli/v2"
)

type Context struct {
	*cli.Context

	k8sClient  *kubernetes.Clientset
	kubeConfig *restclient.Config
	namespace  string
}

type ActionFunc func(ctx *Context) error

// WithK8S is a wrapper for cli.ActionFunc to add kubernetes API capabilities.
func WithK8S(fn ActionFunc) cli.ActionFunc {
	return func(c *cli.Context) error {
		k8sContext := &Context{}
		k8sContext.Context = c
		if err := k8sContext.init(); err != nil {
			return err
		}

		return fn(k8sContext)
	}
}

func (c *Context) init() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return err
	}
	c.kubeConfig = config

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	c.k8sClient = clientset

	clientCfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return err
	}

	namespace := c.String("namespace")
	if namespace == "" {
		namespace = clientCfg.Contexts[clientCfg.CurrentContext].Namespace
	}
	c.namespace = namespace

	return err
}

func (c *Context) Exec(ctx context.Context, namespace, podName string, cmd ...string) error {
	req := c.k8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(c.kubeConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *Context) PortForward(ctx context.Context, namespace, podName, localPort, remotePort string, readyCh chan struct{}) error {
	transport, upgrader, err := spdy.RoundTripperFor(c.kubeConfig)
	if err != nil {
		return err
	}

	reqURL := c.k8sClient.RESTClient().Post().
		Prefix("api/v1/"). // FIXME: prefix is a hacky way to get the correct URL
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").URL()

	stopCh := make(chan struct{})

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, reqURL)
	fw, err := portforward.New(dialer, []string{localPort + ":" + remotePort}, stopCh, readyCh, os.Stdout, os.Stdout)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		stopCh <- struct{}{}
	}()
	if err := fw.ForwardPorts(); err != nil {
		return err
	}

	return nil
}

func (c *Context) Ctx() context.Context {
	return c.Context.Context
}

func (c *Context) Namespace() string {
	return c.namespace
}

func (c *Context) KubeClient() *kubernetes.Clientset {
	return c.k8sClient
}

func (c *Context) KubeConfig() *restclient.Config {
	return c.kubeConfig
}
