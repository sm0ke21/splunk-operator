// Copyright (c) 2018-2021 Splunk Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"

	splcommon "github.com/splunk/splunk-operator/pkg/splunk/common"
	corev1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// kubernetes logger used by splunk.reconcile package
var log = logf.Log.WithName("splunk.reconcile")

// TestResource defines a simple custom resource, used to test the Spec
type TestResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              splcommon.Spec `json:"spec,omitempty"`
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (cr *TestResource) DeepCopyInto(out *TestResource) {
	*out = *cr
	out.TypeMeta = cr.TypeMeta
	cr.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	cr.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestResource.
func (cr *TestResource) DeepCopy() *TestResource {
	if cr == nil {
		return nil
	}
	out := new(TestResource)
	cr.DeepCopyInto(out)
	return out
}

// DeepCopyObject copies the receiver, creating a new runtime.Object.
func (cr *TestResource) DeepCopyObject() runtime.Object {
	if c := cr.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// CreateResource creates a new Kubernetes resource using the REST API.
func CreateResource(client splcommon.ControllerClient, obj splcommon.MetaObject) error {
	scopedLog := log.WithName("CreateResource").WithValues(
		"name", obj.GetObjectMeta().GetName(),
		"namespace", obj.GetObjectMeta().GetNamespace())

	err := client.Create(context.TODO(), obj)

	if err != nil && !errors.IsAlreadyExists(err) {
		scopedLog.Error(err, "Failed to create resource")
		return err
	}

	scopedLog.Info("Created resource")

	return nil
}

// UpdateResource updates an existing Kubernetes resource using the REST API.
func UpdateResource(client splcommon.ControllerClient, obj splcommon.MetaObject) error {
	scopedLog := log.WithName("UpdateResource").WithValues(
		"name", obj.GetObjectMeta().GetName(),
		"namespace", obj.GetObjectMeta().GetNamespace())
	err := client.Update(context.TODO(), obj)

	if err != nil && !errors.IsAlreadyExists(err) {
		scopedLog.Error(err, "Failed to update resource")
		return err
	}

	scopedLog.Info("Updated resource")

	return nil
}

// DeleteResource deletes an existing Kubernetes resource using the REST API.
func DeleteResource(client splcommon.ControllerClient, obj splcommon.MetaObject) error {
	scopedLog := log.WithName("DeleteResource").WithValues(
		"name", obj.GetObjectMeta().GetName(),
		"namespace", obj.GetObjectMeta().GetNamespace())
	err := client.Delete(context.TODO(), obj)

	if err != nil && !errors.IsAlreadyExists(err) {
		scopedLog.Error(err, "Failed to delete resource")
		return err
	}

	scopedLog.Info("Deleted resource")

	return nil
}

// generateHECToken returns a randomly generated HEC token formatted like a UUID.
// Note that it is not strictly a UUID, but rather just looks like one.
func generateHECToken() []byte {
	hecToken := splcommon.GenerateSecret(splcommon.HexBytes, 36)
	hecToken[8] = '-'
	hecToken[13] = '-'
	hecToken[18] = '-'
	hecToken[23] = '-'
	return hecToken
}

// PodExecCommand execute a shell command in the specified pod
func PodExecCommand(c splcommon.ControllerClient, podName string, namespace string, cmd []string, streamOptions *remotecommand.StreamOptions, tty bool, mock bool) (string, string, error) {
	var pod corev1.Pod

	// Get Pod
	namespacedName := types.NamespacedName{Namespace: namespace, Name: podName}
	err := c.Get(context.TODO(), namespacedName, &pod)
	if err != nil {
		return "", "", err
	}

	gvk, _ := apiutil.GVKForObject(&pod, scheme.Scheme)
	var restConfig *rest.Config
	if !mock {
		restConfig, err = config.GetConfig()
		if err != nil {
			return "", "", err
		}
	} else {
		path := os.Getenv("PWD") + "/kubeconfig"
		restConfig, err = clientcmd.BuildConfigFromFlags("", path)
		if err != nil {
			return "", "", err
		}
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, restConfig, serializer.NewCodecFactory(scheme.Scheme))
	if err != nil {
		return "", "", err
	}
	execReq := restClient.Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec")
	option := &corev1.PodExecOptions{
		Command: cmd,
		Stdin:   true,
		Stdout:  true,
		Stderr:  true,
		TTY:     tty,
	}
	if streamOptions == nil {
		option.Stdin = false
	}
	execReq.VersionedParams(
		option,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(restConfig, http.MethodPost, execReq.URL())
	if err != nil {
		return "", "", err
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	streamOptions.Stdout = stdout
	streamOptions.Stderr = stderr

	err = exec.Stream(*streamOptions)

	if err != nil {
		return "", "", err
	}
	return stdout.String(), stderr.String(), nil
}

// PodExecClientImpl is an interface which is used to implement
// PodExecClient to run pod exec commands
// NOTE: This client will be helpful in UTs since we can create
// our own mock client and pass it to the tests to work correctly.
type PodExecClientImpl interface {
	RunPodExecCommand(string) (string, string, error)
}

// blank assignment to implement PodExecClientImpl
var _ PodExecClientImpl = &PodExecClient{}

// PodExecClient implements PodExecClientImpl
type PodExecClient struct {
	client        splcommon.ControllerClient
	cr            splcommon.MetaObject
	targetPodName string
}

// GetPodExecClient returns the client object used to execute pod exec commands
func GetPodExecClient(client splcommon.ControllerClient, cr splcommon.MetaObject, targetPodName string) *PodExecClient {
	return &PodExecClient{
		client:        client,
		cr:            cr,
		targetPodName: targetPodName,
	}
}

// runPodExecCommand runs the commands related to idxc bundle push
func (podExecClient *PodExecClient) RunPodExecCommand(cmd string) (string, string, error) {
	streamOptions := &remotecommand.StreamOptions{
		Stdin: strings.NewReader(cmd),
	}

	return PodExecCommand(podExecClient.client, podExecClient.targetPodName, podExecClient.cr.GetNamespace(), []string{"/bin/sh"}, streamOptions, false, false)
}
