package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelOperation = "sandbox.kce.ksyun.com/operation"

	DataKind       = "kind"
	DataName       = "name"
	DataGeneration = "generation"
	DataAction     = "action"
	DataTemplateID = "templateID"
	DataSandboxID  = "sandboxID"
	DataSandboxIDs = "sandboxIDs"
	DataEndpoint   = "endpoint"
)

type Record struct {
	Namespace  string
	Kind       string
	Name       string
	Generation string
	Action     string
	TemplateID string
	SandboxID  string
	SandboxIDs []string
	Endpoint   string
}

type Recorder struct {
	client client.Client
}

func NewRecorder(c client.Client) *Recorder {
	return &Recorder{client: c}
}

func (r *Recorder) Upsert(ctx context.Context, rec Record) error {
	if rec.Namespace == "" || rec.Kind == "" || rec.Name == "" {
		return fmt.Errorf("operation record requires namespace, kind, and name")
	}
	name := ConfigMapName(rec.Kind, rec.Name)
	data := map[string]string{
		DataKind:       rec.Kind,
		DataName:       rec.Name,
		DataGeneration: rec.Generation,
		DataAction:     rec.Action,
		DataTemplateID: rec.TemplateID,
		DataSandboxID:  rec.SandboxID,
		DataEndpoint:   rec.Endpoint,
	}
	if rec.SandboxIDs != nil {
		b, err := json.Marshal(rec.SandboxIDs)
		if err != nil {
			return err
		}
		data[DataSandboxIDs] = string(b)
	}

	var cm corev1.ConfigMap
	key := client.ObjectKey{Namespace: rec.Namespace, Name: name}
	if err := r.client.Get(ctx, key, &cm); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rec.Namespace,
				Name:      name,
				Labels: map[string]string{
					LabelOperation: "true",
				},
			},
			Data: data,
		}
		return r.client.Create(ctx, &cm)
	}
	cm.Data = data
	if cm.Labels == nil {
		cm.Labels = map[string]string{}
	}
	cm.Labels[LabelOperation] = "true"
	return r.client.Update(ctx, &cm)
}

func (r *Recorder) Get(ctx context.Context, namespace, kind, name string) (*Record, error) {
	var cm corev1.ConfigMap
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ConfigMapName(kind, name)}, &cm); err != nil {
		return nil, err
	}
	rec := &Record{
		Namespace:  namespace,
		Kind:       cm.Data[DataKind],
		Name:       cm.Data[DataName],
		Generation: cm.Data[DataGeneration],
		Action:     cm.Data[DataAction],
		TemplateID: cm.Data[DataTemplateID],
		SandboxID:  cm.Data[DataSandboxID],
		Endpoint:   cm.Data[DataEndpoint],
	}
	if raw := cm.Data[DataSandboxIDs]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &rec.SandboxIDs)
	}
	return rec, nil
}

func (r *Recorder) Delete(ctx context.Context, namespace, kind, name string) error {
	var cm corev1.ConfigMap
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ConfigMapName(kind, name)}, &cm); err != nil {
		return client.IgnoreNotFound(err)
	}
	return r.client.Delete(ctx, &cm)
}

func ConfigMapName(kind, name string) string {
	value := "sandbox-op-" + strings.ToLower(kind) + "-" + name
	value = regexp.MustCompile(`[^a-z0-9.-]+`).ReplaceAllString(strings.ToLower(value), "-")
	value = strings.Trim(value, "-.")
	if len(value) > 253 {
		value = value[:253]
	}
	return value
}
