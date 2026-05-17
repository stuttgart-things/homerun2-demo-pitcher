"""
HTTPRoute (Gateway API) for homerun2-demo-pitcher
"""

import labels

config = labels.config

_hostname = config.httpRouteHostname

httproute = {
    apiVersion: "gateway.networking.k8s.io/v1"
    kind: "HTTPRoute"
    metadata: {
        name: config.name
        namespace: config.namespace
        labels: labels.commonLabels
        if config.annotations or config.httpRouteAnnotations:
            annotations: {
                **config.annotations
                **config.httpRouteAnnotations
            }
    }
    spec: {
        # group + kind are admission-webhook defaults on parentRefs; rendering
        # them explicitly avoids drift on every reconcile (Gateway controller
        # adds them in, ArgoCD compares chart-rendered against live and flags
        # OutOfSync otherwise). Same shape as stuttgart-things/argocd#114
        # fix for the standalone httproute chart.
        parentRefs: [
            {
                group: "gateway.networking.k8s.io"
                kind: "Gateway"
                name: config.httpRouteParentRefName
                if config.httpRouteParentRefNamespace:
                    namespace: config.httpRouteParentRefNamespace
            }
        ]
        if _hostname:
            hostnames: [_hostname]
        rules: [
            {
                matches: [
                    {
                        path: {
                            type: "PathPrefix"
                            value: "/"
                        }
                    }
                ]
                # group + kind + weight are admission defaults on backendRefs;
                # rendering them explicitly prevents the same drift.
                backendRefs: [
                    {
                        group: ""
                        kind: "Service"
                        name: config.name
                        port: config.servicePort
                        weight: 1
                    }
                ]
            }
        ]
    }
} if config.httpRouteEnabled else None
