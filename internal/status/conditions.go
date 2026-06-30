package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	now := metav1.Now()
	next := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
		LastTransitionTime: now,
	}

	for i := range *conditions {
		if (*conditions)[i].Type != conditionType {
			continue
		}
		if (*conditions)[i].Status == status {
			next.LastTransitionTime = (*conditions)[i].LastTransitionTime
		}
		(*conditions)[i] = next
		return
	}
	*conditions = append(*conditions, next)
}
