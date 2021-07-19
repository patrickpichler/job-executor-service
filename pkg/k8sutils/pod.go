package k8sutils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// GetLogsOfPod returns the k8s logs of a job in a namespace
func (k8s *k8sImpl) GetLogsOfPod(jobName string) (string, error) {

	// TODO include the logs of the initcontainer

	podLogOpts := v1.PodLogOptions{}

	list, err := k8s.clientset.CoreV1().Pods(k8s.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil {
		return "", err
	}

	logs := ""

	for _, pod := range list.Items {

		req := k8s.clientset.CoreV1().Pods(k8s.namespace).GetLogs(pod.Name, &podLogOpts)
		podLogs, err := req.Stream(context.TODO())
		if err != nil {
			return "", err
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			return "", err
		}
		logs += buf.String()
	}

	return logs, nil
}

func (k8s *k8sImpl) OpenLogs(jobName string) (io.ReadCloser, error) {
	println("===> openLogs for", jobName)
	pods := k8s.clientset.CoreV1().Pods(k8s.namespace)

	watch, err := pods.Watch(context.TODO(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("job-name", jobName).String(),
	})

	defer watch.Stop()

	if err != nil {
		return nil, fmt.Errorf("noooo: %w", err)
	}

	timeoutChan := make(chan bool)
	var podName string

	go func() {
		time.Sleep(30 * time.Second)
		timeoutChan <- true
	}()

Loop:
	for {
		select {
		case event := <-watch.ResultChan():
			p := event.Object.(*v1.Pod)
			println("===> ", p.Status.Phase)

			for _, c := range p.Status.ContainerStatuses {
				if c.Started != nil && (*c.Started || c.Ready) {
					println("===> container started: ", c.Name)
					podName = p.Name
					println("===> ", podName)
					break Loop
				}
			}

			if p.Status.Phase != v1.PodPending && p.Status.Phase != v1.PodUnknown {
				podName = p.Name
				println("===> ", podName)
				break Loop
			}

		case <-timeoutChan:
			println("===> timeout")
			break Loop
		}
	}

	if podName == "" {
		return nil, fmt.Errorf("error while creating pods it seems")
	}

	podLogOpts := v1.PodLogOptions{
		SinceTime: &metav1.Time{
			Time: time.Unix(0, 0),
		},
		Timestamps: true,
		Follow:     true,
	}

	println("===> get logs for", podName)
	req := pods.GetLogs(podName, &podLogOpts)

	logReader, err := req.Stream(context.TODO())

	if err != nil {
		return nil, err
	}

	return logReader, nil
}
