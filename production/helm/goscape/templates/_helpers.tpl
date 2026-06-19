{{/* Chart name (overridable) */}}
{{- define "goscape.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified app name */}}
{{- define "goscape.fullname" -}}
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

{{/* Chart label */}}
{{- define "goscape.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Common labels */}}
{{- define "goscape.labels" -}}
helm.sh/chart: {{ include "goscape.chart" . }}
{{ include "goscape.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: {{ .Values.deploymentMode | lower }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* Selector labels */}}
{{- define "goscape.selectorLabels" -}}
app.kubernetes.io/name: {{ include "goscape.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* ServiceAccount name */}}
{{- define "goscape.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "goscape.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* Fully qualified image reference */}}
{{- define "goscape.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if .Values.image.registry -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{/* goscape.baseConfig — the generated config.yaml before extraConfig merge */}}
{{- define "goscape.baseConfig" -}}
{{- $mode := .Values.deploymentMode -}}
{{- $g := .Values.goscape -}}
target: all
log_level: {{ $g.logLevel | quote }}
log_format: {{ $g.logFormat | quote }}
ondemand:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "World") }}
  http_listen_network: tcp
  http_listen_address: 0.0.0.0
  http_listen_port: {{ $g.ports.ondemandHTTP }}
  cache_path: {{ $g.cachePath | quote }}
login:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: {{ $g.ports.loginGRPC }}
  sqlite_dsn: {{ printf "%s/login.db" $g.dataPath | quote }}
  save_path: {{ printf "%s/players" $g.dataPath | quote }}
  node_profile: {{ $g.node.profile | quote }}
friends:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: {{ $g.ports.friendsGRPC }}
  sqlite_dsn: {{ printf "%s/friends.db" $g.dataPath | quote }}
  profile: {{ $g.node.profile | quote }}
world:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "World") }}
  tcp_listen_network: tcp
  tcp_listen_address: 0.0.0.0
  tcp_listen_port: {{ $g.ports.worldTCP }}
  node_id: {{ $g.node.id }}
  node_members: {{ $g.node.members }}
  node_profile: {{ $g.node.profile | quote }}
  cache_path: {{ $g.cachePath | quote }}
{{- if eq $mode "SingleBinary" }}
  login_server_enabled: true
  login_server_address: {{ printf "127.0.0.1:%d" (int $g.ports.loginGRPC) | quote }}
  friends_server_enabled: true
  friends_server_address: {{ printf "127.0.0.1:%d" (int $g.ports.friendsGRPC) | quote }}
{{- else if eq $mode "World" }}
  login_server_enabled: true
  login_server_address: {{ required "goscape.loginServerAddress is required when deploymentMode=World" $g.loginServerAddress | quote }}
  friends_server_enabled: true
  friends_server_address: {{ required "goscape.friendsServerAddress is required when deploymentMode=World" $g.friendsServerAddress | quote }}
{{- end }}
{{- end -}}

{{/* goscape.config — baseConfig with extraConfig deep-merged over it */}}
{{- define "goscape.config" -}}
{{- $base := include "goscape.baseConfig" . | fromYaml -}}
{{- $extra := deepCopy (.Values.goscape.extraConfig | default dict) -}}
{{- mergeOverwrite $base $extra | toYaml -}}
{{- end -}}
