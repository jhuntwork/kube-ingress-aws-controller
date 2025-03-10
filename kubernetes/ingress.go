package kubernetes

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type ingressList struct {
	Kind       string              `json:"kind"`
	APIVersion string              `json:"apiVersion"`
	Metadata   ingressListMetadata `json:"metadata"`
	Items      []*ingress          `json:"items"`
}

type ingress struct {
	Metadata kubeItemMetadata `json:"metadata"`
	Spec     ingressSpec      `json:"spec"`
	Status   ingressStatus    `json:"status"`
}

type ingressListMetadata struct {
	SelfLink        string `json:"selfLink"`
	ResourceVersion string `json:"resourceVersion"`
}

type kubeItemMetadata struct {
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	UID               string            `json:"uid"`
	Annotations       map[string]string `json:"annotations"`
	SelfLink          string            `json:"selfLink"`
	ResourceVersion   string            `json:"resourceVersion"`
	Generation        int               `json:"generation"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	DeletionTimestamp time.Time         `json:"deletionTimestamp"`
	Labels            map[string]string `json:"labels"`
}

type ingressSpec struct {
	Rules            []ingressItemRule `json:"rules"`
	IngressClassName string            `json:"ingressClassName"`
}

type ingressItemRule struct {
	Host string `json:"host"`
}

type ingressStatus struct {
	LoadBalancer ingressLoadBalancerStatus `json:"loadBalancer"`
}

type ingressLoadBalancerStatus struct {
	Ingress []ingressLoadBalancer `json:"ingress"`
}

type ingressLoadBalancer struct {
	Hostname string `json:"hostname"`
}

const (
	// ingressALBIPAddressType is used in external-dns, https://github.com/kubernetes-incubator/external-dns/pull/1079
	ingressALBIPAddressType            = "alb.ingress.kubernetes.io/ip-address-type"
	IngressAPIVersionExtensions        = "extensions/v1beta1"
	IngressAPIVersionNetworking        = "networking.k8s.io/v1beta1"
	IngressAPIVersionNetworkingV1      = "networking.k8s.io/v1"
	ingressListResource                = "/apis/%s/ingresses"
	ingressPatchStatusResource         = "/apis/%s/namespaces/%s/ingresses/%s/status"
	ingressCertificateARNAnnotation    = "zalando.org/aws-load-balancer-ssl-cert"
	ingressSchemeAnnotation            = "zalando.org/aws-load-balancer-scheme"
	ingressSharedAnnotation            = "zalando.org/aws-load-balancer-shared"
	ingressSecurityGroupAnnotation     = "zalando.org/aws-load-balancer-security-group"
	ingressSSLPolicyAnnotation         = "zalando.org/aws-load-balancer-ssl-policy"
	ingressLoadBalancerTypeAnnotation  = "zalando.org/aws-load-balancer-type"
	ingressHTTP2Annotation             = "zalando.org/aws-load-balancer-http2"
	ingressWAFWebACLIDAnnotation       = "zalando.org/aws-waf-web-acl-id"
	ingressNLBExtraListenersAnnotation = "zalando.org/aws-nlb-extra-listeners"
	ingressClassAnnotation             = "kubernetes.io/ingress.class"
)

func getAnnotationsString(annotations map[string]string, key string, defaultValue string) string {
	if val, ok := annotations[key]; ok {
		return val
	}
	return defaultValue
}

func getIngressClassName(spec ingressSpec, defaultValue string) string {
	if spec.IngressClassName != "" {
		return spec.IngressClassName
	}
	return defaultValue
}

type ingressClient struct {
	apiVersion string
}

func (ic *ingressClient) listIngress(c client) (*ingressList, error) {
	r, err := c.get(fmt.Sprintf(ingressListResource, ic.apiVersion))
	if err != nil {
		return nil, fmt.Errorf("failed to get ingress list: %w", err)
	}

	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var result ingressList
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

type patchIngressStatus struct {
	Status ingressStatus `json:"status"`
}

func (ic *ingressClient) updateIngressLoadBalancer(c client, ns, name, newHostName string) error {
	patchStatus := patchIngressStatus{
		Status: ingressStatus{
			LoadBalancer: ingressLoadBalancerStatus{
				Ingress: []ingressLoadBalancer{{Hostname: newHostName}},
			},
		},
	}

	resource := fmt.Sprintf(ingressPatchStatusResource, ic.apiVersion, ns, name)
	payload, err := json.Marshal(patchStatus)
	if err != nil {
		return err
	}

	r, err := c.patch(resource, payload)
	if err != nil {
		return fmt.Errorf("failed to patch ingress %s/%s = %q: %w", ns, name, newHostName, err)
	}
	defer r.Close()
	return nil
}
