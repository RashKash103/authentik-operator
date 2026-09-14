{{/*
Chart name, overridable with `nameOverride`. Truncated to 63 chars because it
ends up in label values, which Kubernetes caps at 63.
*/}}
{{- define "authentik-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name used for every generated resource name.
`fullnameOverride` wins outright; otherwise `<release>-<chart>`, collapsed to
just `<release>` when the release name already contains the chart name.
*/}}
{{- define "authentik-operator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Chart name and version, as required for the `helm.sh/chart` label.
*/}}
{{- define "authentik-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every object in the release.
*/}}
{{- define "authentik-operator.labels" -}}
helm.sh/chart: {{ include "authentik-operator.chart" . }}
{{ include "authentik-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "authentik-operator.name" . }}
app.kubernetes.io/component: controller-manager
{{- end }}

{{/*
Selector labels. Immutable across upgrades: they land in
`Deployment.spec.selector`, which Kubernetes refuses to let you change.
*/}}
{{- define "authentik-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "authentik-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the ServiceAccount the Deployment should use.
*/}}
{{- define "authentik-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "authentik-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Fully qualified image reference. A digest, when set, wins over the tag so the
deployed bits are immutable.
*/}}
{{- define "authentik-operator.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) }}
{{- end }}
{{- end }}

{{/*
The metrics bind address passed to the manager. controller-runtime treats the
literal "0" as "metrics disabled".
*/}}
{{- define "authentik-operator.metricsBindAddress" -}}
{{- if .Values.metrics.enabled }}
{{- printf ":%v" .Values.metrics.port }}
{{- else }}
{{- print "0" }}
{{- end }}
{{- end }}

{{/*
Scheme the metrics endpoint speaks, derived from `metrics.secure` unless the
user pinned `metrics.serviceMonitor.scheme` explicitly.
*/}}
{{- define "authentik-operator.metricsScheme" -}}
{{- if .Values.metrics.serviceMonitor.scheme }}
{{- .Values.metrics.serviceMonitor.scheme }}
{{- else if .Values.metrics.secure }}
{{- print "https" }}
{{- else }}
{{- print "http" }}
{{- end }}
{{- end }}
