package exec

import (
	"bytes"
	"io"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
)

// IExec is an injectable interface for running remote exec commands.
type IExec interface {
	// ExecCommandInPod exec cmd in pod set.
	ExecCommandInPod(pod *corev1.Pod, cmd ...string) error
}

type remoteExec struct {
	restGVKClient rest.Interface
	config        *rest.Config
}

// NewRemoteExec returns a new IExec which will exec remote cmd.
func NewRemoteExec(restGVKClient rest.Interface, config *rest.Config) IExec {
	return &remoteExec{
		restGVKClient: restGVKClient,
		config:        config,
	}
}

// ExecOptions passed to ExecWithOptions.
type ExecOptions struct {
	Command []string

	Namespace     string
	PodName       string
	ContainerName string

	Stdin         io.Reader
	CaptureStdout bool
	CaptureStderr bool
	// If false, whitespace in std{err,out} will be removed.
	PreserveWhitespace bool
}

// ExecCommandInPod implements IExec interface.
func (e *remoteExec) ExecCommandInPod(pod *corev1.Pod, cmd ...string) error {
	if err := e.ExecCommandInContainer(pod.Namespace, pod.Name, pod.Spec.Containers[0].Name, cmd...); err != nil {
		return err
	}
	return nil
}

// ExecCommandInContainer executes a command in the specified container.
func (e *remoteExec) ExecCommandInContainer(namespace, podName, containerName string, cmd ...string) error {
	stdout, stderr, err := e.ExecCommandInContainerWithFullOutput(namespace, podName, containerName, cmd...)
	if err != nil {
		klog.Errorf("Failed to execute command, Command: %s \nStdout: %s \nStderr: %s \nerr: %v", cmd, stdout, stderr, err)
		return err
	}
	klog.V(2).Infof("Successfully executed the command, Command: %s \nStdout: %s", cmd, stdout)
	return nil
}

// ExecCommandInContainerWithFullOutput executes a command in the
// specified container and return stdout, stderr and error
func (e *remoteExec) ExecCommandInContainerWithFullOutput(namespace, podName, containerName string, cmd ...string) (string, string, error) {
	return e.ExecWithOptions(ExecOptions{
		Command:       cmd,
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: containerName,

		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})
}

// ExecWithOptions executes a command in the specified container,
// returning stdout, stderr and error. `options` allowed for
// additional parameters to be passed.
func (e *remoteExec) ExecWithOptions(options ExecOptions) (string, string, error) {
	const tty = false

	req := e.restGVKClient.Post().
		Resource("pods").
		Name(options.PodName).
		Namespace(options.Namespace).
		SubResource("exec").
		Param("container", options.ContainerName)

	req.VersionedParams(&corev1.PodExecOptions{
		Container: options.ContainerName,
		Command:   options.Command,
		Stdin:     options.Stdin != nil,
		Stdout:    options.CaptureStdout,
		Stderr:    options.CaptureStderr,
		TTY:       tty,
	}, scheme.ParameterCodec)

	var stdout, stderr bytes.Buffer
	err := execute("POST", req.URL(), e.config, options.Stdin, &stdout, &stderr, tty)

	if options.PreserveWhitespace {
		return stdout.String(), stderr.String(), err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func execute(method string, url *url.URL, config *rest.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		klog.Errorf("Failed to init executor: %v", err)
		return err
	}
	if err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	}); err != nil {
		return err
	}
	return nil
}
