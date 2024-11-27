# Istio Installation

## 1. Add Istio repository and update
helm repo add istio https://istio-release.storage.googleapis.com/charts
helm repo update

## 2. Create and label namespace
kubectl create namespace istio-system
kubectl label namespace istio-system istio-injection=enabled

## 3. Install Istio components
# Install base Istio CRDs and services
helm install istio-base istio/base -n istio-system --set defaultRevision=default

# Install Istio control plane
helm install istiod istio/istiod -n istio-system --wait

## 4. Install Ingress Gateway (optional but recommended)
kubectl create namespace istio-ingress
kubectl label namespace istio-ingress istio-injection=enabled
helm install istio-ingressgateway istio/gateway -n istio-ingress

## 5. Verify installation
kubectl get pods -n istio-system
kubectl get svc -n istio-system
kubectl get pods -n istio-ingress

## 6. Enable Istio injection for other namespaces (optional)
# Add this for each namespace where you want Istio enabled
# kubectl label namespace <namespace> istio-injection=enabled

## 7. Install addons (optional)
# Prometheus for metrics
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/prometheus.yaml

# Grafana for visualization
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/grafana.yaml

# Kiali for service mesh visualization
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/kiali.yaml

# Jaeger for tracing
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/jaeger.yaml
