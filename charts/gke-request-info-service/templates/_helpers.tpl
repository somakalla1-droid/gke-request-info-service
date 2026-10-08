{{- define "app.name" -}}{{- .Chart.Name | trunc 63 | trimSuffix "-" }}{{- end }}
{{- define "app.fullname" -}}{{- printf "%s-%s" .Release.Name (include "app.name" .) | trunc 63 | trimSuffix "-" }}{{- end }}
{{- define "app.labels" -}}
app.kubernetes.io/name: {{ include "app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
{{- define "app.serviceAccount" -}}{{ default (include "app.fullname" .) .Values.serviceAccount.name }}{{- end }}
{{- define "app.configMap" -}}{{ printf "%s-config" (include "app.fullname" .) }}{{- end }}
{{- define "app.secret" -}}{{ default (printf "%s-secrets" (include "app.fullname" .)) .Values.secret.existingSecret }}{{- end }}
