# Scheduling shared Tasks

Our Lighthouse loader accepts `lighthouse.jenkins-x.io/taskScheduling` on a shared
Task's metadata. It carries only `nodeSelector` and `tolerations`:

```yaml
metadata:
  name: ros-abb-test
  annotations:
    lighthouse.jenkins-x.io/taskScheduling: |
      nodeSelector:
        cloud.google.com/gke-nodepool: tekton-abb-linux
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
      tolerations:
        - key: node-type
          operator: Equal
          value: tekton-abb-linux
          effect: NoSchedule
```

Lighthouse copies these requirements into the corresponding PipelineRun
`taskRunSpecs[].podTemplate` after resolving shared Task files. Future callers need
only their usual Task reference and test parameters. Existing per-test CPU/memory
requests, annotations and other Pod settings are preserved. Unannotated Tasks are
unchanged.

The shared Task's selector values override matching caller keys, including old
node-pool selections. Unrelated keys are retained. Required tolerations are added
without removing existing entries or duplicating identical ones. Tekton continues
to merge its global Pod template normally. Incompatible affinity constraints are
not rewritten: those must be reviewed by the caller.

Unknown annotation fields, duplicate keys, malformed YAML and empty requirements
fail PipelineRun loading. The same handling supports inline Tasks and inline
`finally` Tasks. Unresolved cluster Task references are outside this loader's scope.

This is a **NoMagic Lighthouse extension**, not a Tekton Task API field. Deploy
the updated Lighthouse webhooks image before using the annotation: older versions
ignore it. Provision the selected node pool before enabling dependent pipelines.
This source change does not deploy anything.

[Tekton Pod template documentation](https://tekton.dev/docs/pipelines/podtemplates/)
describes the generated TaskRun scheduling fields.
