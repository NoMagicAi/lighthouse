package inrepo

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/pod"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

const taskSchedulingAnnotation = "lighthouse.jenkins-x.io/taskScheduling"

// Shared Tasks own these scheduling requirements; per-test resources stay on the caller.
// See docs/task-scheduling.md for precedence and rollout requirements.
func applyTaskScheduling(pr *pipelinev1.PipelineRun) error {
	if pr.Spec.PipelineSpec == nil {
		return nil
	}
	for _, tasks := range [][]pipelinev1.PipelineTask{pr.Spec.PipelineSpec.Tasks, pr.Spec.PipelineSpec.Finally} {
		for _, task := range tasks {
			if task.TaskSpec == nil {
				continue
			}
			encoded, present := task.TaskSpec.Metadata.Annotations[taskSchedulingAnnotation]
			if !present {
				continue
			}
			var scheduling struct {
				NodeSelector map[string]string   `json:"nodeSelector"`
				Tolerations  []corev1.Toleration `json:"tolerations"`
			}
			if err := yaml.UnmarshalStrict([]byte(encoded), &scheduling); err != nil {
				return fmt.Errorf("task %s: invalid %s: %w", task.Name, taskSchedulingAnnotation, err)
			}
			if len(scheduling.NodeSelector) == 0 && len(scheduling.Tolerations) == 0 {
				return fmt.Errorf("task %s: empty %s", task.Name, taskSchedulingAnnotation)
			}
			index := slices.IndexFunc(pr.Spec.TaskRunSpecs, func(spec pipelinev1.PipelineTaskRunSpec) bool {
				return spec.PipelineTaskName == task.Name
			})
			if index == -1 {
				index = len(pr.Spec.TaskRunSpecs)
				pr.Spec.TaskRunSpecs = append(pr.Spec.TaskRunSpecs, pipelinev1.PipelineTaskRunSpec{PipelineTaskName: task.Name})
			}
			spec := &pr.Spec.TaskRunSpecs[index]
			if spec.PodTemplate == nil {
				spec.PodTemplate = &pod.PodTemplate{}
			}
			if spec.PodTemplate.NodeSelector == nil {
				spec.PodTemplate.NodeSelector = map[string]string{}
			}
			for key, value := range scheduling.NodeSelector {
				spec.PodTemplate.NodeSelector[key] = value
			}
			for _, tolerance := range scheduling.Tolerations {
				found := slices.ContainsFunc(spec.PodTemplate.Tolerations, func(existing corev1.Toleration) bool {
					return reflect.DeepEqual(existing, tolerance)
				})
				if !found {
					spec.PodTemplate.Tolerations = append(spec.PodTemplate.Tolerations, tolerance)
				}
			}
		}
	}
	return nil
}
