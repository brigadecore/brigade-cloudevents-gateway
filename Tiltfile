if 'ENABLE_NGROK_EXTENSION' in os.environ and os.environ['ENABLE_NGROK_EXTENSION'] == '1':
  v1alpha1.extension_repo(
    name = 'default',
    url = 'https://github.com/tilt-dev/tilt-extensions'
  )
  v1alpha1.extension(name = 'ngrok', repo_name = 'default', repo_path = 'ngrok')

load('ext://min_k8s_version', 'min_k8s_version')
min_k8s_version('1.18.0')

trigger_mode(TRIGGER_MODE_MANUAL)

load('ext://namespace', 'namespace_create')
namespace_create('brigade-cloudevents-gateway')
k8s_resource(
  new_name = 'namespace',
  objects = ['brigade-cloudevents-gateway:namespace'],
  labels = ['brigade-cloudevents-gateway']
)


docker_build(
  'brigadecore/brigade-cloudevents-gateway', '.',
  only = [
    'internal/',
    'config.go',
    'go.mod',
    'go.sum',
    'main.go'
  ],
  ignore = ['**/*_test.go']
)
k8s_resource(
  workload = 'brigade-cloudevents-gateway',
  new_name = 'gateway',
  port_forwards = '31700:8080',
  labels = ['brigade-cloudevents-gateway']
)
k8s_resource(
  workload = 'gateway',
  objects = [
    'brigade-cloudevents-gateway-config:secret',
    'brigade-cloudevents-gateway:secret'
  ]
)

k8s_yaml(
  helm(
    './charts/brigade-cloudevents-gateway',
    name = 'brigade-cloudevents-gateway',
    namespace = 'brigade-cloudevents-gateway',
    set = [
      'brigade.apiToken=' + os.environ['BRIGADE_API_TOKEN'],
      'tls.enabled=false'
    ]
  )
)
