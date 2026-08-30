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
{{- $repo := .Values.image.repository -}}
{{- if .Values.image.registry -}}{{- $repo = printf "%s/%s" .Values.image.registry $repo -}}{{- end -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repo .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $repo (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
{{- end -}}

{{/* goscape.baseConfig — the generated config.yaml before extraConfig merge */}}
{{- define "goscape.baseConfig" -}}
{{- $mode := .Values.deploymentMode -}}
{{- $g := .Values.goscape -}}
{{- $acct := $g.account -}}
{{- $stateful := or (eq $mode "SingleBinary") (eq $mode "Management") -}}
{{- if and $acct.enabled (not $stateful) -}}
{{- fail "goscape.account.enabled requires deploymentMode SingleBinary or Management: the portal is a client of the central database, which World mode does not render" -}}
{{- end -}}
{{- if and $acct.gameLogin (not $acct.enabled) -}}
{{- fail "goscape.account.gameLogin requires goscape.account.enabled: login would be pointed at an AccountService that this release does not run" -}}
{{- end -}}
target: all
log_level: {{ $g.logLevel | quote }}
log_format: {{ $g.logFormat | quote }}
ondemand:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "World") }}
  http_listen_network: tcp
  http_listen_address: 0.0.0.0
  http_listen_port: {{ $g.ports.ondemandHTTP }}
  cache_path: {{ $g.cachePath | quote }}
  # ondemand keeps its own copy of the world's identity — the two modules do
  # not cross-import — and /rs2.cgi advertises it to the Java applet, which
  # then connects on node_port. Mirror both from one set of values so the
  # applet is never handed a world id or port the world does not answer on.
  node_id: {{ $g.node.id }}
  node_members: {{ $g.node.members }}
  node_port: {{ $g.ports.worldTCP }}
{{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
database:
{{- if eq $g.database.backend "postgres" }}
  backend: postgres
  postgres:
    dsn: {{ printf "postgres://%s:${GOSCAPE_DB_PASSWORD}@%s:%d/%s?sslmode=%s" $g.database.postgres.user (required "goscape.database.postgres.host is required when backend=postgres" $g.database.postgres.host) (int $g.database.postgres.port) $g.database.postgres.database $g.database.postgres.sslmode | quote }}
{{- else }}
  backend: sqlite
  sqlite:
    dsn: {{ printf "%s/goscape.db" $g.dataPath | quote }}
{{- end }}
{{- end }}
login:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: {{ $g.ports.loginGRPC }}
  save_path: {{ printf "%s/players" $g.dataPath | quote }}
  node_profile: {{ $g.node.profile | quote }}
{{- if and $acct.gameLogin $stateful }}
  # Portal-password game login. auto_register must go off with it: the two are
  # a validation conflict, because accounts are created in the portal rather
  # than on first game login.
  auth_mode: account
  account_grpc_address: {{ printf "127.0.0.1:%d" (int $g.ports.accountGRPC) | quote }}
  auto_register: false
{{- end }}
friends:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: {{ $g.ports.friendsGRPC }}
hiscore:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  http_listen_network: tcp
  http_listen_address: 0.0.0.0
  http_listen_port: {{ $g.ports.hiscoreHTTP }}
  profile: {{ $g.node.profile | quote }}
  trust_gateway_headers: {{ .Values.hiscoreGateway.createGatewayConfig }}
{{- if and $acct.enabled $stateful }}
account:
  enable: true
  http_listen_address: 0.0.0.0
  http_listen_port: {{ $g.ports.accountHTTP }}
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: {{ $g.ports.accountGRPC }}
  public_url: {{ required "goscape.account.publicUrl is required when goscape.account.enabled (it is the base of every emailed link and the OAuth redirect)" $acct.publicUrl | quote }}
  character_limit: {{ $acct.characterLimit }}
  gate:
    providers:
      {{- toYaml $acct.gate.providers | nindent 6 }}
  smtp:
    host: {{ $acct.smtp.host | quote }}
    port: {{ $acct.smtp.port }}
    from: {{ $acct.smtp.from | quote }}
    username: {{ $acct.smtp.username | quote }}
    {{- if $acct.existingSecret }}
    password: "${GOSCAPE_ACCOUNT_SMTP_PASSWORD}"
    {{- end }}
  providers:
    discord:
      client_id: {{ $acct.providers.discord.clientId | quote }}
      {{- if $acct.existingSecret }}
      client_secret: "${GOSCAPE_ACCOUNT_DISCORD_CLIENT_SECRET}"
      {{- end }}
  {{- if $acct.existingSecret }}
  admin_token: "${GOSCAPE_ACCOUNT_ADMIN_TOKEN}"
  {{- end }}
{{- end }}
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
{{- $g := $ctx.Values.goscape -}}
{{- $pgActive := and (eq $g.database.backend "postgres") (or (eq $mode "SingleBinary") (eq $mode "Management")) -}}
{{- $acct := $g.account -}}
{{- $acctActive := and $acct.enabled (or (eq $mode "SingleBinary") (eq $mode "Management")) -}}
{{- $acctSecret := and $acctActive $acct.existingSecret -}}
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
  # Also set at pod level, not just on the ServiceAccount: with
  # serviceAccount.create=false the chart does not own the SA object, so the
  # pod-level field is the only thing that keeps the token unmounted.
  automountServiceAccountToken: {{ $ctx.Values.serviceAccount.automountServiceAccountToken }}
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
        - "--config.expand-env=true"
        {{- with $w.extraArgs }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- if or $pgActive $acctSecret $w.extraEnv }}
      env:
        {{- if $pgActive }}
        - name: GOSCAPE_DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: {{ required "goscape.database.postgres.existingSecret is required when backend=postgres" $g.database.postgres.existingSecret }}
              key: {{ $g.database.postgres.secretKey }}
        {{- end }}
        {{- if $acctSecret }}
        {{- /* optional: true is what lets one Secret carry only the keys this
               deployment actually uses — an absent key leaves the variable
               unset, config expansion turns it into the empty string, and
               empty is already each field's "feature off" value. */}}
        - name: GOSCAPE_ACCOUNT_ADMIN_TOKEN
          valueFrom:
            secretKeyRef:
              name: {{ $acct.existingSecret }}
              key: admin-token
              optional: true
        - name: GOSCAPE_ACCOUNT_SMTP_PASSWORD
          valueFrom:
            secretKeyRef:
              name: {{ $acct.existingSecret }}
              key: smtp-password
              optional: true
        - name: GOSCAPE_ACCOUNT_DISCORD_CLIENT_SECRET
          valueFrom:
            secretKeyRef:
              name: {{ $acct.existingSecret }}
              key: discord-client-secret
              optional: true
        {{- end }}
        {{- with $w.extraEnv }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
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
        - name: hiscore-http
          containerPort: {{ $ctx.Values.goscape.ports.hiscoreHTTP }}
        {{- end }}
        {{- if $acctActive }}
        - name: account-http
          containerPort: {{ $ctx.Values.goscape.ports.accountHTTP }}
        - name: account-grpc
          containerPort: {{ $ctx.Values.goscape.ports.accountGRPC }}
        {{- end }}
      readinessProbe:
        {{- if eq $mode "Management" }}
        {{- /* Management runs login-grpc/friends-grpc/hiscore-http — no
               ondemand HTTP port exists to httpGet /healthz against (the
               hiscore listener serves the API only, not /healthz), so this
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
      {{- if $w.livenessProbe.enabled }}
      livenessProbe:
        tcpSocket:
          {{- if eq $mode "Management" }}
          port: login-grpc
          {{- else }}
          port: world-tcp
          {{- end }}
        initialDelaySeconds: {{ $w.livenessProbe.initialDelaySeconds }}
        periodSeconds: {{ $w.livenessProbe.periodSeconds }}
        failureThreshold: {{ $w.livenessProbe.failureThreshold }}
      {{- end }}
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
        - name: tmp
          mountPath: /tmp
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
    - name: tmp
      emptyDir: {}
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
