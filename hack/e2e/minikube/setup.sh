#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
MANIFESTS_DIR="${SCRIPT_DIR}/../manifests"

PROFILE_NAME="barnacle-e2e"
KUBERNETES_VERSION="v1.34.0"
CPUS=4
MEMORY=16384
DISK_SIZE="200g"
NODES=4
REGISTRY_PORT=30500

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v minikube &> /dev/null; then
        log_error "minikube is not installed. Please install minikube first."
        exit 1
    fi

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Please install kubectl first."
        exit 1
    fi

    if ! command -v docker &> /dev/null; then
        log_error "docker is not installed. Please install docker first."
        exit 1
    fi

    if ! command -v kustomize &> /dev/null; then
        log_warn "kustomize is not installed. Attempting to install..."
        install_kustomize
    fi

    log_info "All prerequisites satisfied."
}

install_kustomize() {
    log_info "Installing kustomize..."

    # Use the official kustomize installation script
    KUSTOMIZE_VERSION="v5.4.1"
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "${INSTALL_DIR}"

    # Download and install
    curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- "${KUSTOMIZE_VERSION#v}" "${INSTALL_DIR}"

    # Add to PATH for this session if needed
    if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
        export PATH="${INSTALL_DIR}:${PATH}"
    fi

    if ! command -v kustomize &> /dev/null; then
        log_error "Failed to install kustomize. Please install manually:"
        log_error "  curl -s 'https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh' | bash"
        exit 1
    fi

    log_info "kustomize installed successfully."
}

create_cluster() {
    log_info "Creating minikube cluster with profile: ${PROFILE_NAME}"

    # Check if cluster already exists
    if minikube status -p "${PROFILE_NAME}" &> /dev/null; then
        log_warn "Cluster ${PROFILE_NAME} already exists. Skipping creation."
        return 0
    fi

    minikube start \
        --profile="${PROFILE_NAME}" \
        --kubernetes-version="${KUBERNETES_VERSION}" \
        --cpus="${CPUS}" \
        --memory="${MEMORY}" \
        --disk-size="${DISK_SIZE}" \
        --driver=docker \
        --nodes="${NODES}" \
        --insecure-registry="192.168.0.0/16"

    log_info "Cluster created successfully."

    # Enable ingress addons
    log_info "Enabling ingress addons..."
    minikube addons enable ingress -p "${PROFILE_NAME}"
    minikube addons enable ingress-dns -p "${PROFILE_NAME}"
    log_info "Ingress addons enabled."

    # the ingress addon is actually bugged with multi-replica. We need to recreate the pod for now
    minikube kubectl -p "${PROFILE_NAME}" -- delete pod -n kube-system kube-ingress-dns-minikube
    minikube kubectl -p "${PROFILE_NAME}" -- apply -f "${MANIFESTS_DIR}/ingress-dns/kube-ingress-dns-minikube.yaml"

    # Wait for ingress controller to be ready
    log_info "Waiting for ingress dns..."
    minikube kubectl -p "${PROFILE_NAME}" -- wait --for=condition=ready --timeout=120s pod/kube-ingress-dns-minikube -n kube-system
}

configure_ingress_dns() {
    log_info "Configuring ingress DNS..."

    MINIKUBE_IP=$(minikube ip -p "${PROFILE_NAME}")

    # Check if systemd-resolved is in use
    if systemctl is-active --quiet systemd-resolved; then
        log_info "Configuring systemd-resolved for .test domain..."

        sudo mkdir -p /etc/systemd/resolved.conf.d
        sudo tee /etc/systemd/resolved.conf.d/minikube.conf > /dev/null << EOF
[Resolve]
DNS=${MINIKUBE_IP}
Domains=~test
FallbackDNS=8.8.8.8 1.1.1.1
DefaultRoute=false
EOF
        sudo systemctl restart systemd-resolved
        log_info "systemd-resolved configured."

    # Check if NetworkManager is in use
    elif systemctl is-active --quiet NetworkManager; then
        log_info "Configuring NetworkManager for .test domain..."

        sudo mkdir -p /etc/NetworkManager/dnsmasq.d/
        echo "server=/test/${MINIKUBE_IP}" | sudo tee /etc/NetworkManager/dnsmasq.d/minikube.conf > /dev/null
        sudo systemctl restart NetworkManager
        log_info "NetworkManager configured."

    else
        log_warn "Could not detect DNS resolver. Please configure manually."
        log_warn "Add the following to your DNS configuration:"
        log_warn "  DNS server: ${MINIKUBE_IP}"
        log_warn "  Domain: .test"
    fi

    log_info "Ingress DNS configuration complete."
    log_info "Registry will be available at: registry.test"
}

deploy_registry() {
    log_info "Deploying registry using kustomize..."
    kubectl config use-context "${PROFILE_NAME}"

    # Apply namespace first
    kubectl apply -f "${MANIFESTS_DIR}/namespace.yaml"

    # Deploy registry using kustomize
    kubectl apply -k "${MANIFESTS_DIR}/registry/"

    # Wait for ingress controller to be ready
    log_info "Waiting for ingress controller..."
    kubectl wait --for=condition=available --timeout=120s deployment/ingress-nginx-controller -n ingress-nginx || true

    kubectl wait --for=condition=available --timeout=120s deployment/registry -n barnacle-e2e
    log_info "Registry is ready."
}

build_barnacle_image() {
    log_info "Building and pushing barnacle image to in-cluster registry..."

    # Build the image and tag for localhost (Docker trusts localhost as insecure)
    docker build -t "localhost:5000/barnacle:e2e" "${PROJECT_ROOT}"

    # Port-forward to the registry service (localhost is trusted by Docker)
    kubectl port-forward -n barnacle-e2e svc/registry 5000:5000 &
    PF_PID=$!

    # Wait for port-forward to establish
    sleep 3

    # Push via localhost
    docker push "localhost:5000/barnacle:e2e"

    # Cleanup port-forward
    kill $PF_PID 2>/dev/null || true

    log_info "Barnacle image pushed successfully."
}

apply_manifests() {
    log_info "Applying Kubernetes manifests using kustomize..."

    # Set kubectl context
    kubectl config use-context "${PROFILE_NAME}"

    # Get registry URL for image reference
    MINIKUBE_IP=$(minikube ip -p "${PROFILE_NAME}")
    REGISTRY_URL="${MINIKUBE_IP}:${REGISTRY_PORT}"

    # Use kustomize to set the barnacle image with the registry URL
    log_info "Setting barnacle image to: ${REGISTRY_URL}/barnacle:e2e"
    pushd "${MANIFESTS_DIR}" > /dev/null
    kustomize edit set image "barnacle=${REGISTRY_URL}/barnacle:e2e"
    popd > /dev/null

    # Apply all manifests using kustomize
    log_info "Applying manifests..."
    kubectl apply -k "${MANIFESTS_DIR}"

    log_info "Manifests applied successfully."
}

wait_for_deployments() {
    log_info "Waiting for deployments to be ready..."

    kubectl config use-context "${PROFILE_NAME}"

    log_info "Waiting for redis..."
    kubectl wait --for=condition=available --timeout=120s deployment/redis -n barnacle-e2e

    log_info "Waiting for barnacle..."
    kubectl wait --for=condition=available --timeout=120s deployment/barnacle -n barnacle-e2e

    log_info "All deployments are ready."
}

verify_health() {
    log_info "Verifying barnacle health via ingress..."

    kubectl config use-context "${PROFILE_NAME}"

    # Wait for ingress to be ready
    log_info "Waiting for barnacle ingress to be ready..."
    sleep 5

    # Check health endpoint via ingress
    local retries=10
    local count=0
    while [ $count -lt $retries ]; do
        if curl -sf http://barnacle.test/health > /dev/null 2>&1; then
            log_info "Barnacle is healthy!"
            log_info "Health verification complete."
            return 0
        fi
        count=$((count + 1))
        log_info "Waiting for barnacle to be reachable via ingress (attempt $count/$retries)..."
        sleep 3
    done

    log_error "Barnacle health check failed after $retries attempts."
    exit 1
}

print_status() {
    log_info "Cluster status:"
    echo ""
    minikube status -p "${PROFILE_NAME}"
    echo ""

    log_info "Pods in barnacle-e2e namespace:"
    kubectl get pods -n barnacle-e2e
    echo ""

    log_info "Services in barnacle-e2e namespace:"
    kubectl get svc -n barnacle-e2e
    echo ""

    log_info "Setup complete!"
    echo ""
    echo "To use this cluster:"
    echo "  kubectl config use-context ${PROFILE_NAME}"
    echo ""
    echo "Services available via ingress:"
    echo "  Barnacle:  http://barnacle.test"
    echo "  Registry:  http://registry.test"
    echo ""
    echo "Example usage:"
    echo "  # Check barnacle health"
    echo "  curl http://barnacle.test/health"
    echo ""
    echo "  # Push image to registry"
    echo "  docker push registry.test/myimage:tag"
    echo ""
    echo "  # Pull image through barnacle"
    echo "  docker pull barnacle.test/local/myimage:tag"
}

teardown() {
    log_info "Tearing down e2e cluster..."

    if minikube status -p "${PROFILE_NAME}" &> /dev/null; then
        minikube delete -p "${PROFILE_NAME}"
        log_info "Cluster deleted."
    else
        log_warn "Cluster ${PROFILE_NAME} does not exist."
    fi
}

usage() {
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  setup     Create cluster, build image, and deploy (default)"
    echo "  build     Build barnacle image only"
    echo "  deploy    Apply manifests only"
    echo "  status    Show cluster status"
    echo "  teardown  Delete the cluster"
    echo "  help      Show this help message"
}

main() {
    local command="${1:-setup}"

    case "${command}" in
        setup)
            check_prerequisites
            create_cluster
            configure_ingress_dns
            deploy_registry
            build_barnacle_image
            apply_manifests
            wait_for_deployments
            verify_health
            print_status
            ;;
        build)
            check_prerequisites
            build_barnacle_image
            ;;
        deploy)
            check_prerequisites
            apply_manifests
            wait_for_deployments
            verify_health
            print_status
            ;;
        status)
            print_status
            ;;
        teardown)
            teardown
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: ${command}"
            usage
            exit 1
            ;;
    esac
}

main "$@"
