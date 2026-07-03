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
world:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "World") }}
  tcp_listen_network: tcp
  tcp_listen_address: 0.0.0.0
  tcp_listen_port: {{ $g.ports.worldTCP }}
  node_id: {{ $g.node.id }}
  node_members: {{ $g.node.members }}
  node_profile: {{ $g.node.profile | quote }}
  cache_path: {{ $g.cachePath | quote }}
{{- if and $g.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
  rsa_private_key_path: {{ printf "/etc/goscape-login-rsa/%s" $g.loginRsaKey.key | quote }}
{{- end }}
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

{{/* goscape.podTemplate — the pod template body (metadata + spec) shared by all workloads.
     Call with (dict "ctx" $ "workload" $w). */}}
{{- define "goscape.podTemplate" -}}
{{- $ctx := .ctx -}}
{{- $w := .workload -}}
{{- $mode := $ctx.Values.deploymentMode -}}
metadata:
  annotations:
    checksum/config: {{ include "goscape.config" $ctx | sha256sum }}
    {{- with $w.podAnnotations }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  labels:
    {{- include "goscape.selectorLabels" $ctx | nindent 4 }}
    {{- with $w.podLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  serviceAccountName: {{ include "goscape.serviceAccountName" $ctx }}
  {{- with $ctx.Values.image.pullSecrets }}
  imagePullSecrets:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with $w.podSecurityContext }}
  securityContext:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  containers:
    - name: goscape
      image: {{ include "goscape.image" $ctx }}
      imagePullPolicy: {{ $ctx.Values.image.pullPolicy }}
      args:
        - "--config.file=/etc/goscape/config.yaml"
        {{- with $w.extraArgs }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- with $w.extraEnv }}
      env:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      ports:
        {{- if or (eq $mode "SingleBinary") (eq $mode "World") }}
        - name: ondemand-http
          containerPort: {{ $ctx.Values.goscape.ports.ondemandHTTP }}
        - name: world-tcp
          containerPort: {{ $ctx.Values.goscape.ports.worldTCP }}
        {{- end }}
        {{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
        - name: login-grpc
          containerPort: {{ $ctx.Values.goscape.ports.loginGRPC }}
        - name: friends-grpc
          containerPort: {{ $ctx.Values.goscape.ports.friendsGRPC }}
        {{- end }}
      readinessProbe:
        {{- if eq $mode "Management" }}
        {{- /* Management runs only login-grpc/friends-grpc — no ondemand
               HTTP port exists to httpGet /healthz against, so this
               deployment keeps the coarser tcpSocket check (arch-29.6). */}}
        tcpSocket:
          port: login-grpc
        {{- else }}
        {{- /* SingleBinary and World both always run ondemand alongside
               world (see the ports block above), so /healthz's tick-
               liveness check is available and replaces the old tcpSocket
               probe, which only proved the TCP listener was up — not
               that the tick loop was still moving (arch-29.6). */}}
        httpGet:
          path: /healthz
          port: ondemand-http
        {{- end }}
        initialDelaySeconds: 10
        periodSeconds: 10
      {{- with $w.containerSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $w.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        - name: config
          mountPath: /etc/goscape
        {{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
        - name: login-rsa
          mountPath: /etc/goscape-login-rsa
          readOnly: true
        {{- end }}
        {{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
        - name: data
          mountPath: {{ $ctx.Values.goscape.dataPath }}
        {{- end }}
  {{- with $w.nodeSelector }}
  nodeSelector:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with $w.tolerations }}
  tolerations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with $w.affinity }}
  affinity:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  volumes:
    - name: config
      configMap:
        name: {{ include "goscape.fullname" $ctx }}
    {{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
    - name: login-rsa
      secret:
        secretName: {{ $ctx.Values.goscape.loginRsaKey.existingSecret | quote }}
    {{- end }}
    {{- if and (or (eq $mode "SingleBinary") (eq $mode "Management")) (not $ctx.Values.persistence.enabled) }}
    - name: data
      emptyDir: {}
    {{- end }}
{{- end -}}
