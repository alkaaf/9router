# Domain 6: Kubernetes Deployment Manifests

## Overview

Create Kubernetes manifests for deploying 9Router with PostgreSQL. Includes a
PostgreSQL StatefulSet (for self-hosted deployments; managed PG like RDS/Cloud SQL
is the production recommendation), a 9Router Deployment with 2-3 replicas, Services,
ConfigMaps, Secrets, and PVC templates.

The manifests assume `POSTGRES_URL` is provided via a K8s Secret. Health checks
probe `/api/health` on the app container.

## Scope

- **In scope**: StatefulSet, Deployment, Services (ClusterIP + optional LoadBalancer),
  Secret (POSTGRES_URL), PVC, ConfigMap, health probes, resource limits
- **Out of scope**: Helm chart (can wrap these), managed PG provisioning (AWS RDS,
  GCP Cloud SQL, Aiven), TLS cert management (assume TLS termination at ingress)

## Files Affected

| File | Action | LOC Change |
|------|--------|------------|
| `deploy/k8s/postgres-statefulset.yaml` | CREATE | ~80 |
| `deploy/k8s/postgres-service.yaml` | CREATE | ~20 |
| `deploy/k8s/postgres-pvc.yaml` | CREATE | ~25 |
| `deploy/k8s/9router-deployment.yaml` | CREATE | ~80 |
| `deploy/k8s/9router-service.yaml` | CREATE | ~20 |
| `deploy/k8s/9router-secret.yaml` | CREATE | ~15 |
| `deploy/k8s/9router-configmap.yaml` | CREATE | ~15 |

## Dependencies

Domain 2 (schema) must be complete — manifests reference the schema init behavior.
Domain 3 (driver) must be complete — manifests set POSTGRES_URL.

## Sub-Tasks

1. **Create PostgreSQL StatefulSet (`postgres-statefulset.yaml`)**
   - Description: StatefulSet with 1 replica, `postgres:16-alpine` image, resource
     requests (100m CPU, 256Mi memory), limits (500m CPU, 1Gi memory). Environment
     from Secret: POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD. Volume mount for
     PVC at `/var/lib/postgresql/data`. Liveness probe: `pg_isready`. Readiness
     probe: `pg_isready` after 5s initial delay. Startup probe to allow DB init time.
   - Acceptance: `kubectl apply -f postgres-statefulset.yaml` creates a running
     PostgreSQL pod. `kubectl exec` into pod → `psql` connects successfully.
   - Risk: Low

2. **Create PostgreSQL Service (`postgres-service.yaml`)**
   - Description: Headless Service (`clusterIP: None`) for StatefulSet DNS
     (`postgres-0.postgres.default.svc.cluster.local`). Port 5432.
   - Acceptance: `nslookup postgres-0.postgres` from another pod resolves.
   - Risk: Low

3. **Create PVC template (`postgres-pvc.yaml`)**
   - Description: `PersistentVolumeClaim` with `storageClassName` (configurable),
     50Gi request, `ReadWriteOnce` access mode. Tied to StatefulSet via
     `volumeClaimTemplates`.
   - Acceptance: PVC binds to a volume. Data persists across pod restarts.
   - Risk: Low

4. **Create 9Router Deployment (`9router-deployment.yaml`)**
   - Description: Deployment with 2-3 replicas, resource requests (200m CPU, 512Mi
     memory), limits (1 CPU, 2Gi memory). Env from ConfigMap + Secret:
     `POSTGRES_URL` from Secret, `POSTGRES_POOL_SIZE=10`, `NODE_ENV=production`.
     Port 20128 (HTTP). Liveness probe: `GET /api/health` (initialDelay 30s,
     period 10s). Readiness probe: same (initialDelay 5s, period 5s).
     `terminationGracePeriodSeconds: 30` for flush on shutdown.
   - Acceptance: `kubectl apply` creates 2-3 pods. All pods pass readiness.
     `kubectl port-forward` → dashboard loads. API calls work.
   - Risk: Low

5. **Create 9Router Service (`9router-service.yaml`)**
   - Description: ClusterIP Service on port 20128 targeting port 20128. Optional
     LoadBalancer variant for external access.
   - Acceptance: Service has cluster IP. Other pods in the namespace can reach
     `9router:20128`.
   - Risk: Low

6. **Create Secret (`9router-secret.yaml`)**
   - Description: K8s Secret with `POSTGRES_URL` key. Value: `postgres://9router:<password>@postgres-0.postgres:5432/9router`. Document that for production, use Sealed Secrets or External Secrets Operator.
   - Acceptance: Secret is created. Mounted as env var in Deployment. Not visible
     in `kubectl describe pod` (masked).
   - Risk: Low

7. **Create ConfigMap (`9router-configmap.yaml`)**
   - Description: Non-sensitive config: `POSTGRES_POOL_SIZE=10`, `LOG_LEVEL=info`,
     `PORT=20128`.
   - Acceptance: ConfigMap values appear as env vars in pods.
   - Risk: Low

8. **Create deploy script / README**
   - Description: `deploy/k8s/README.md` with step-by-step deploy instructions:
     `kubectl apply -f postgres-pvc.yaml` → `kubectl apply -f postgres-*.yaml` →
     `kubectl apply -f 9router-*.yaml`. Include `kubectl logs`, `kubectl exec`
     debugging steps.
   - Acceptance: Following README deploys a working 9Router + PostgreSQL stack
     from scratch on a fresh K8s cluster.
   - Risk: Low

## Effort Estimate

3-5 days

## Risk Level

Low — standard K8s manifests; no custom operators required.

## Testing Requirements

- Unit tests: N/A (YAML manifests)
- Integration tests: Deploy to kind/minikube, verify end-to-end
- Manual testing: Full deploy cycle: PVC → StatefulSet → Service → Secret →
  ConfigMap → Deployment → Service. Verify dashboard, API calls, usage tracking.
