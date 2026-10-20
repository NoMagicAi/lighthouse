package main

import (
	"testing"
	"time"

	"github.com/jenkins-x/lighthouse/pkg/apis/lighthouse/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExpired(t *testing.T) {
	now := time.Now()
	maxAge := 168 * time.Hour
	recent := metav1.NewTime(now.Add(-time.Minute))
	old := metav1.NewTime(now.Add(-200 * time.Hour))

	tests := []struct {
		name string
		job  v1alpha1.LighthouseJob
		want bool
	}{
		{
			name: "new job the controller has not started yet",
			job:  v1alpha1.LighthouseJob{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: recent}},
			want: false,
		},
		{
			name: "old job that never started",
			job:  v1alpha1.LighthouseJob{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: old}},
			want: true,
		},
		{
			name: "running job",
			job: v1alpha1.LighthouseJob{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: recent},
				Status:     v1alpha1.LighthouseJobStatus{StartTime: recent},
			},
			want: false,
		},
		{
			name: "job completed recently",
			job: v1alpha1.LighthouseJob{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: old},
				Status:     v1alpha1.LighthouseJobStatus{StartTime: old, CompletionTime: &recent},
			},
			want: false,
		},
		{
			name: "job completed long ago",
			job: v1alpha1.LighthouseJob{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: old},
				Status:     v1alpha1.LighthouseJobStatus{StartTime: old, CompletionTime: &old},
			},
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := expired(&test.job, now, maxAge); got != test.want {
				t.Errorf("expired() = %v, want %v", got, test.want)
			}
		})
	}
}
