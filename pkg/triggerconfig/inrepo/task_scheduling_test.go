package inrepo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSharedTaskScheduling(t *testing.T) {
	data := []byte(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: shared-task-scheduling
spec:
  taskRunTemplate:
    podTemplate:
      nodeSelector:
        zone: existing-zone
        pool: global-pool
      tolerations:
        - key: global
          operator: Exists
  taskRunSpecs:
    - pipelineTaskName: simtest
      metadata:
        annotations:
          existing: retained
      podTemplate:
        nodeSelector:
          pool: old-pool
        tolerations:
          - key: existing
            operator: Exists
      stepSpecs:
        - name: test
          computeResources:
            requests:
              cpu: "16"
  pipelineSpec:
    tasks:
      - name: simtest
        taskSpec:
          metadata:
            annotations:
              lighthouse.jenkins-x.io/taskScheduling: |
                nodeSelector:
                  pool: simulation
                tolerations:
                  - key: dedicated
                    operator: Equal
                    value: simulation
                    effect: NoSchedule
          steps:
            - name: test
              image: example.invalid/test
      - name: unrelated
        taskSpec:
          steps:
            - name: build
              image: example.invalid/build
`)
	pr, err := LoadTektonResourceAsPipelineRun(&UsesResolver{}, data)
	require.NoError(t, err)
	require.Len(t, pr.Spec.TaskRunSpecs, 1)
	spec := pr.Spec.TaskRunSpecs[0]
	assert.Equal(t, "simulation", spec.PodTemplate.NodeSelector["pool"])
	assert.Equal(t, "retained", spec.Metadata.Annotations["existing"])
	require.Len(t, spec.StepSpecs, 1)
	assert.Equal(t, "16", spec.StepSpecs[0].ComputeResources.Requests.Cpu().String())
	require.Len(t, spec.PodTemplate.Tolerations, 2)
	assert.Equal(t, "existing", spec.PodTemplate.Tolerations[0].Key)
	assert.Equal(t, "dedicated", spec.PodTemplate.Tolerations[1].Key)
	assert.Equal(t, "global-pool", pr.Spec.TaskRunTemplate.PodTemplate.NodeSelector["pool"])
	require.NoError(t, applyTaskScheduling(pr))
	assert.Len(t, pr.Spec.TaskRunSpecs[0].PodTemplate.Tolerations, 2)

	for _, invalid := range []string{"nodeSelector: []", "nodeSelektor: {}", "{}", "nodeSelector: {pool: a, pool: b}"} {
		pr.Spec.PipelineSpec.Tasks[0].TaskSpec.Metadata.Annotations[taskSchedulingAnnotation] = invalid
		assert.ErrorContains(t, applyTaskScheduling(pr), "task simtest:")
	}

	// A new caller has no taskRunSpecs to maintain, including finally Tasks.
	pr, err = LoadTektonResourceAsPipelineRun(&UsesResolver{}, data)
	require.NoError(t, err)
	pr.Spec.PipelineSpec.Finally = pr.Spec.PipelineSpec.Tasks[:1]
	pr.Spec.PipelineSpec.Tasks = pr.Spec.PipelineSpec.Tasks[1:]
	pr.Spec.TaskRunSpecs = nil
	require.NoError(t, applyTaskScheduling(pr))
	require.Len(t, pr.Spec.TaskRunSpecs, 1)
	assert.Equal(t, "simulation", pr.Spec.TaskRunSpecs[0].PodTemplate.NodeSelector["pool"])
}
