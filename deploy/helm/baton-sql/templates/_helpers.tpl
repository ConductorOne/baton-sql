{{/* Expand the name of the chart. */}}
{{- define "baton-sql.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified app name. */}}
{{- define "baton-sql.fullname" -}}
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

{{- define "baton-sql.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "baton-sql.labels" -}}
helm.sh/chart: {{ include "baton-sql.chart" . }}
{{ include "baton-sql.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end -}}

{{- define "baton-sql.selectorLabels" -}}
app.kubernetes.io/name: {{ include "baton-sql.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "baton-sql.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "baton-sql.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "baton-sql.configMapName" -}}
{{- if .Values.config.existingConfigMap -}}
{{- .Values.config.existingConfigMap -}}
{{- else -}}
{{- printf "%s-config" (include "baton-sql.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "baton-sql.secretName" -}}
{{- if .Values.secret.existingSecret -}}
{{- .Values.secret.existingSecret -}}
{{- else -}}
{{- printf "%s-secrets" (include "baton-sql.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "baton-sql.filesSecretName" -}}
{{- if .Values.files.existingSecret -}}
{{- .Values.files.existingSecret -}}
{{- else -}}
{{- printf "%s-files" (include "baton-sql.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/* Returns "true" when a files Secret should be mounted, empty string otherwise. */}}
{{- define "baton-sql.filesEnabled" -}}
{{- if or .Values.files.existingSecret (gt (len .Values.files.data) 0) -}}true{{- end -}}
{{- end -}}

{{/*
Emit env entries pulling from the chart-managed or user-supplied Secret.
No-op when neither secret.create nor secret.existingSecret is set.
*/}}
{{- define "baton-sql.secretEnv" -}}
{{- $hasSecret := or .Values.secret.create .Values.secret.existingSecret -}}
{{- if $hasSecret -}}
{{- $secretName := include "baton-sql.secretName" . -}}
{{- $clientIdKey := "BATON_CLIENT_ID" -}}
{{- $clientSecretKey := "BATON_CLIENT_SECRET" -}}
{{- if .Values.secret.existingSecret -}}
{{- $clientIdKey = default $clientIdKey .Values.secret.existingSecretKeys.clientId -}}
{{- $clientSecretKey = default $clientSecretKey .Values.secret.existingSecretKeys.clientSecret -}}
{{- end }}
- name: BATON_CLIENT_ID
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: {{ $clientIdKey }}
      optional: true
- name: BATON_CLIENT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: {{ $clientSecretKey }}
      optional: true
{{- if .Values.secret.existingSecret }}
{{- range $envName, $key := .Values.secret.existingSecretExtra }}
- name: {{ $envName }}
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: {{ $key }}
{{- end }}
{{- else }}
{{- range $envName, $_ := .Values.secret.extra }}
- name: {{ $envName }}
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: {{ $envName }}
{{- end }}
{{- end }}
{{- end -}}
{{- end -}}
