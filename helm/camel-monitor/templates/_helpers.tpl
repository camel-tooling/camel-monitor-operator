{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "camel-monitor.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "camel-monitor.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "camel-monitor.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "camel-monitor.labels" -}}
helm.sh/chart: {{ include "camel-monitor.chart" . }}
{{ include "camel-monitor.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "camel-monitor.selectorLabels" -}}
app.kubernetes.io/name: {{ include "camel-monitor.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Operator container image reference.
If operator.image already carries a tag or a digest (e.g. "repo/img:1.2.3" or
"repo/img@sha256:..."), it is used as is, for backward compatibility.
Otherwise the tag is operator.tag, defaulting to the chart appVersion, so that a
released chart deploys the matching operator image instead of "latest".
*/}}
{{- define "camel-monitor.operatorImage" -}}
{{- $image := .Values.operator.image -}}
{{- $lastSegment := last (splitList "/" $image) -}}
{{- if or (contains "@" $image) (contains ":" $lastSegment) -}}
{{- $image -}}
{{- else -}}
{{- $defaultTag := .Chart.AppVersion | toString -}}
{{- if hasSuffix "-SNAPSHOT" $defaultTag -}}
{{- /* development versions are not published, nightly builds are pushed as latest */ -}}
{{- $defaultTag = "latest" -}}
{{- end -}}
{{- printf "%s:%s" $image (.Values.operator.tag | default $defaultTag | toString) -}}
{{- end -}}
{{- end -}}
