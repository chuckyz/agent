---
aliases:
- /docs/agent/latest/flow/reference/components/discovery.kubernetes
title: discovery.kubernetes
---

# discovery.kubernetes

`discovery.kubernetes` allows you to find scrape targets from Kubernetes
resources. It watches cluster state, and ensures targets are continually synced
with what is currently running in your cluster.

If you supply no connection information, this component defaults to an
in-cluster config. A kubeconfig file or manual connection settings can be used
to override the defaults.

## Usage

```river
discovery.kubernetes "LABEL" {
  role = "DISCOVERY_ROLE"
}
```

## Arguments

The following arguments are supported:

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`api_server` | `string` | URL of Kubernetes API server. | | no
`role` | `string` | Type of Kubernetes resource to query. | | **yes**
`kubeconfig_file` | `string` | Path of kubeconfig file to use for connecting to Kubernetes. | | no

The `role` argument is required to specify what type of targets to discover.
`role` must be one of `node`, `pod`, `service`, `endpoints`, `endpointslice`,
or `ingress`.

### node role

The `node` role discovers one target per cluster node with the address
defaulting to the HTTP port of the Kubelet daemon. The target address defaults
to the first existing address of the Kubernetes node object in the address type
order of `NodeInternalIP`, `NodeExternalIP`, `NodeLegacyHostIP`, and
`NodeHostName`.

The following labels are included for discovered nodes:

* `__meta_kubernetes_node_name`: The name of the node object.
* `__meta_kubernetes_node_provider_id`: The cloud provider's name for the node object.
* `__meta_kubernetes_node_label_<labelname>`: Each label from the node object.
* `__meta_kubernetes_node_labelpresent_<labelname>`: Set to `true` for each label from the node object.
* `__meta_kubernetes_node_annotation_<annotationname>`: Each annotation from the node object.
* `__meta_kubernetes_node_annotationpresent_<annotationname>`: Set to `true`
  for each annotation from the node object.
* `__meta_kubernetes_node_address_<address_type>`: The first address for each
  node address type, if it exists.

In addition, the `instance` label for the node will be set to the node name as
retrieved from the API server.

### service role

The `service` role discovers a target for each service port for each service.
This is generally useful for externally monitoring a service. The address will
be set to the Kubernetes DNS name of the service and respective service port.

The following labels are included for discovered services:

* `__meta_kubernetes_namespace`: The namespace of the service object.
* `__meta_kubernetes_service_annotation_<annotationname>`: Each annotation from
  the service object.
* `__meta_kubernetes_service_annotationpresent_<annotationname>`: `true` for
  each annotation of the service object.
* `__meta_kubernetes_service_cluster_ip`: The cluster IP address of the
  service. This does not apply to services of type `ExternalName`.
* `__meta_kubernetes_service_external_name`: The DNS name of the service.
  This only applies to services of type `ExternalName`.
* `__meta_kubernetes_service_label_<labelname>`: Each label from the service
  object.
* `__meta_kubernetes_service_labelpresent_<labelname>`: `true` for each label
  of the service object.
* `__meta_kubernetes_service_name`: The name of the service object.
* `__meta_kubernetes_service_port_name`: Name of the service port for the
  target.
* `__meta_kubernetes_service_port_number`: Number of the service port for the
  target.
* `__meta_kubernetes_service_port_protocol`: Protocol of the service port for
  the target.
* `__meta_kubernetes_service_type`: The type of the service.

### pod role

The `pod` role discovers all pods and exposes their containers as targets. For
each declared port of a container, a single target is generated.

If a container has no specified ports, a port-free target per container is
created. These targets must have a port manually injected using a
[`discovery.relabel` component][discovery.relabel] before metrics can be
collected from them.

The following labels are included for discovered pods:

* `__meta_kubernetes_namespace`: The namespace of the pod object.
* `__meta_kubernetes_pod_name`: The name of the pod object.
* `__meta_kubernetes_pod_ip`: The pod IP of the pod object.
* `__meta_kubernetes_pod_label_<labelname>`: Each label from the pod object.
* `__meta_kubernetes_pod_labelpresent_<labelname>`: `true` for each label from
  the pod object.
* `__meta_kubernetes_pod_annotation_<annotationname>`: Each annotation from the
  pod object.
* `__meta_kubernetes_pod_annotationpresent_<annotationname>`: `true` for each
  annotation from the pod object.
* `__meta_kubernetes_pod_container_init`: `true` if the container is an
  `InitContainer`.
* `__meta_kubernetes_pod_container_name`: Name of the container the target
  address points to.
* `__meta_kubernetes_pod_container_image`: The image the container is using.
* `__meta_kubernetes_pod_container_port_name`: Name of the container port.
* `__meta_kubernetes_pod_container_port_number`: Number of the container port.
* `__meta_kubernetes_pod_container_port_protocol`: Protocol of the container
  port.
* `__meta_kubernetes_pod_ready`: Set to `true` or `false` for the pod's ready
  state.
* `__meta_kubernetes_pod_phase`: Set to `Pending`, `Running`, `Succeeded`, `Failed` or
  `Unknown` in the lifecycle.
* `__meta_kubernetes_pod_node_name`: The name of the node the pod is scheduled
  onto.
* `__meta_kubernetes_pod_host_ip`: The current host IP of the pod object.
* `__meta_kubernetes_pod_uid`: The UID of the pod object.
* `__meta_kubernetes_pod_controller_kind`: Object kind of the pod controller.
* `__meta_kubernetes_pod_controller_name`: Name of the pod controller.

### endpoints role

The `endpoints` role discovers targets from listed endpoints of a service. For
each endpoint address one target is discovered per port. If the endpoint is
backed by a pod, all container ports of a pod are discovered as targets even if
they are not bound to an endpoint port.

The following labels are included for discovered endpoints:

* `__meta_kubernetes_namespace:` The namespace of the endpoints object.
* `__meta_kubernetes_endpoints_name:` The names of the endpoints object.
* `__meta_kubernetes_endpoints_label_<labelname>`: Each label from the
  endpoints object.
* `__meta_kubernetes_endpoints_labelpresent_<labelname>`: `true` for each label
  from the endpoints object.
* The following labels are attached for all targets discovered directly from
  the endpoints list:
  * `__meta_kubernetes_endpoint_hostname`: Hostname of the endpoint.
  * `__meta_kubernetes_endpoint_node_name`: Name of the node hosting the
    endpoint.
  * `__meta_kubernetes_endpoint_ready`: Set to `true` or `false` for the
    endpoint's ready state.
  * `__meta_kubernetes_endpoint_port_name`: Name of the endpoint port.
  * `__meta_kubernetes_endpoint_port_protocol`: Protocol of the endpoint port.
  * `__meta_kubernetes_endpoint_address_target_kind`: Kind of the endpoint
    address target.
  * `__meta_kubernetes_endpoint_address_target_name`: Name of the endpoint
    address target.
* If the endpoints belong to a service, all labels of the `service` role
  discovery are attached.
* For all targets backed by a pod, all labels of the `pod` role discovery are
  attached.

### endpointslice role

The endpointslice role discovers targets from existing Kubernetes endpoint
slices. For each endpoint address referenced in the `EndpointSlice` object, one
target is discovered. If the endpoint is backed by a pod, all container ports
of a pod are discovered as targets even if they are not bound to an endpoint
port.

The following labels are included for discovered endpoint slices:

* `__meta_kubernetes_namespace`: The namespace of the endpoints object.
* `__meta_kubernetes_endpointslice_name`: The name of endpoint slice object.
* The following labels are attached for all targets discovered directly from
  the endpoint slice list:
  * `__meta_kubernetes_endpointslice_address_target_kind`: Kind of the
    referenced object.
  * `__meta_kubernetes_endpointslice_address_target_name`: Name of referenced
    object.
  * `__meta_kubernetes_endpointslice_address_type`: The IP protocol family of
    the address of the target.
  * `__meta_kubernetes_endpointslice_endpoint_conditions_ready`: Set to `true`
    or `false` for the referenced endpoint's ready state.
  * `__meta_kubernetes_endpointslice_endpoint_topology_kubernetes_io_hostname`:
    Name of the node hosting the referenced endpoint.
  * `__meta_kubernetes_endpointslice_endpoint_topology_present_kubernetes_io_hostname`:
    `true` if the referenced object has a `kubernetes.io/hostname` annotation.
  * `__meta_kubernetes_endpointslice_port`: Port of the referenced endpoint.
  * `__meta_kubernetes_endpointslice_port_name`: Named port of the referenced
    endpoint.
  * `__meta_kubernetes_endpointslice_port_protocol`: Protocol of the referenced
    endpoint.
* If the endpoints belong to a service, all labels of the `service` role
  discovery are attached.
* For all targets backed by a pod, all labels of the `pod` role discovery are
  attached.

### ingress role

The `ingress` role discovers a target for each path of each ingress. This is
generally useful for externally monitoring an ingress. The address will be set
to the host specified in the Kubernetes `Ingress`'s `spec` block.

The following labels are included for discovered ingress objects:

* `__meta_kubernetes_namespace`: The namespace of the ingress object.
* `__meta_kubernetes_ingress_name`: The name of the ingress object.
* `__meta_kubernetes_ingress_label_<labelname>`: Each label from the ingress
  object.
* `__meta_kubernetes_ingress_labelpresent_<labelname>`: `true` for each label
  from the ingress object.
* `__meta_kubernetes_ingress_annotation_<annotationname>`: Each annotation from
  the ingress object.
* `__meta_kubernetes_ingress_annotationpresent_<annotationname>`: `true` for each
  annotation from the ingress object.
* `__meta_kubernetes_ingress_class_name`: Class name from ingress spec, if
  present.
* `__meta_kubernetes_ingress_scheme`: Protocol scheme of ingress, `https` if TLS
  config is set. Defaults to `http`.
* `__meta_kubernetes_ingress_path`: Path from ingress spec. Defaults to /.

## Blocks

The following blocks are supported inside the definition of
`discovery.kubernetes`:

Hierarchy | Block | Description | Required
--------- | ----- | ----------- | --------
namespaces | [namespaces][] | Information about which Kubernetes namespaces to search. | no
selectors | [selectors][] | Information about which Kubernetes namespaces to search. | no
http_client_config | [http_client_config][] | HTTP client configuration for Kubernetes requests. | no
http_client_config > basic_auth | [basic_auth][] | Configure basic_auth for authenticating to the endpoint. | no
http_client_config > authorization | [authorization][] | Configure generic authorization to the endpoint. | no
http_client_config > oauth2 | [oauth2][] | Configure OAuth2 for authenticating to the endpoint. | no
http_client_config > oauth2 > tls_config | [tls_config][] | Configure TLS settings for connecting to the endpoint. | no

The `>` symbol indicates deeper levels of nesting. For example,
`http_client_config > basic_auth` refers to a `basic_auth` block defined inside
an `http_client_config` block.

[namespaces]: #namespaces-block
[selectors]: #selectors-block
[http_client_config]: #http_client_config-block
[basic_auth]: #basic_auth-block
[authorization]: #authorization-block
[oauth2]: #oauth2-block
[tls_config]: #tls_config-block

### namespaces block

The `namespaces` block limits the namespaces to discover resources in. If
omitted, all namespaces are searched.

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`own_namespace` | `bool`   | Include the namespace the agent is running in. | | no
`names` | `[]string` | List of namespaces to search. | | no

### selectors block

The `selectors` block contains optional label and field selectors to limit the
discovery process to a subset of resources.

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`role` | `string`   | Role of the selector. | | yes
`label`| `string`   | Label selector string. | | no
`field` | `string`   | Field selector string. | | no

See Kubernetes' documentation for [Field selectors][] and [Labels and
selectors][] to learn more about the possible filters that can be used.

> **Note**: Using multiple `discovery.kubernetes` components with different
> selectors may result in a bigger load against the Kubernetes API.
>
> Selectors are recommended for retrieving a small set of resources in a very
> large cluster. Smaller clusters are recommended to avoid selectors in favor
> of filtering with [a `discovery.relabel` component][discovery.relabel]
> instead.

[Field selectors]: https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/
[Labels and selectros]: https://Kubernetes.io/docs/concepts/overview/working-with-objects/labels/
[discovery.relabel]: {{< relref "./discovery.relabel.md" >}}

### http_client_config block

The `http_client_config` block configures settings used to connect to the
Kubernetes API server.

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`bearer_token` | `secret` | Bearer token to authenticate with. | | no
`bearer_token_file` | `string` | File containing a bearer token to authenticate with. | | no
`proxy_url` | `string` | HTTP proxy to proxy requests through. | | no
`follow_redirects` | `bool` | Whether redirects returned by the server should be followed. | `true` | no
`enable_http_2` | `bool` | Whether HTTP2 is supported for requests. | `true` | no

`bearer_token`, `bearer_token_file`, `basic_auth`, `authorization`, and
`oauth2` are mutually exclusive and only one can be provided inside of a
`http_client_config` block.

The following sub-blocks are supported for `http_client_config`:

Name | Description | Required
---- | ----------- | --------
[`basic_auth`](#basic_auth-block) | Configure basic_auth for authenticating against Kubernetes. | no
[`authorization`](#authorization-block) | Configure generic authorization against Kubernetes. | no
[`oauth2`](#oauth2-block) | Configure OAuth2 for authenticating against Kubernetes. | no
[`tls_config`](#tls_config-block) | Configure TLS settings for connecting to Kubernetes. | no

### basic_auth block

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`username` | `string` | Basic auth username. | | no
`password` | `secret` | Basic auth password. | | no
`password_file` | `string` | File containing the basic auth password. | | no

`password` and `password_file` are mututally exclusive and only one can be
provided inside of a `basic_auth` block.

### authorization block

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`type` | `string` | Authorization type, for example, "Bearer". | | no
`credential` | `secret` | Secret value. | | no
`credentials_file` | `string` | File containing the secret value. | | no

`credential` and `credentials_file` are mututally exclusive and only one can be
provided inside of an `authorization` block.

### oauth2 block

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`client_id` | `string` | OAuth2 client ID. | | no
`client_secret` | `secret` | OAuth2 client secret. | | no
`client_secret_file` | `string` | File containing the OAuth2 client secret. | | no
`scopes` | `list(string)` | List of scopes to authenticate with. | | no
`token_url` | `string` | URL to fetch the token from. | | no
`endpoint_params` | `map(string)` | Optional parameters to append to the token URL. | | no
`proxy_url` | `string` | Optional proxy URL for OAuth2 requests. | | no

`client_secret` and `client_secret_file` are mututally exclusive and only one
can be provided inside of an `oauth2` block.

The `oauth2` block may also contain its own separate `tls_config` sub-block.

### tls_config block

Name | Type | Description | Default | Required
---- | ---- | ----------- | ------- | --------
`ca_file` | `string` | CA certificate to validate the server with. | | no
`cert_file` | `string` | Certificate file for client authentication. | | no
`key_file` | `string` | Key file for client authentication. | | no
`server_name` | `string` | ServerName extension to indicate the name of the server. | | no
`insecure_skip_verify` | `bool` | Disables validation of the server certificate. | | no
`min_version` | `string` | Minimum acceptable TLS version. | | no

When `min_version` is not provided, the minumum acceptable TLS version is
inherited from Go's default minimum version, TLS 1.2. If `min_version` is
provided, it must be set to one of the following strings:

* `"TLS10"` (TLS 1.0)
* `"TLS11"` (TLS 1.1)
* `"TLS12"` (TLS 1.2)
* `"TLS13"` (TLS 1.3)

## Exported fields

The following fields are exported and can be referenced by other components:

Name | Type | Description
---- | ---- | -----------
`targets` | `list(map(string))` | The set of targets discovered from the Kubernetes API.

## Component health

`discovery.kubernetes` is only be reported as unhealthy when given an invalid
configuration. In those cases, exported fields retain their last healthy
values.

## Debug information

`discovery.kubernetes` does not expose any component-specific debug information.

### Debug metrics

`discovery.kubernetes` does not expose any component-specific debug metrics.

## Examples

### In-cluster discovery

This example uses in-cluster authentication to discover all pods:

```river
discovery.kubernetes "k8s_pods" {
  role = "pod"
}
```

### Kubeconfig authentication

This example uses a kubeconfig file to authenticate to the Kubernetes API:

```river
discovery.kubernetes "k8s_pods" {
  role = "pod"
  kubeconfig_file = "/etc/k8s/kubeconfig.yaml"
}
```

### Limit searched namespaces

This example limits the namespaces where pods are discovered using the `namespaces` block:

```river
discovery.kubernetes "k8s_pods" {
  role = "pod"
  namespaces {
    names = ["myapp"]
  }
}
```
