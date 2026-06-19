.DEFAULT_GOAL := lint
.PHONY: lint template install-single-binary install-management install-world uninstall

CHART := .
NAMESPACE ?= goscape

lint:
	helm lint $(CHART) -f single-binary-values.yaml
	helm lint $(CHART) -f management-values.yaml
	helm lint $(CHART) -f world-values.yaml \
		--set goscape.loginServerAddress=mgmt:2004 \
		--set goscape.friendsServerAddress=mgmt:2005

template:
	helm template goscape $(CHART) -f single-binary-values.yaml

install-single-binary:
	helm upgrade --install goscape $(CHART) -f single-binary-values.yaml \
		--create-namespace --namespace $(NAMESPACE)

install-management:
	helm upgrade --install goscape-mgmt $(CHART) -f management-values.yaml \
		--create-namespace --namespace $(NAMESPACE)

# Override RELEASE and the management addresses per world.
RELEASE ?= goscape-world-1
LOGIN_ADDR ?= goscape-mgmt:2004
FRIENDS_ADDR ?= goscape-mgmt:2005
install-world:
	helm upgrade --install $(RELEASE) $(CHART) -f world-values.yaml \
		--namespace $(NAMESPACE) \
		--set goscape.loginServerAddress=$(LOGIN_ADDR) \
		--set goscape.friendsServerAddress=$(FRIENDS_ADDR)

uninstall:
	helm uninstall goscape --namespace $(NAMESPACE) || true
