# Camel Monitor Operator

The Camel Monitor Operator is a project created to simplify the management of any Camel application on a Kubernetes cluster. The tool is in charge to monitor any Camel application and provide a set of basic information, useful to learn how your fleet of Camel (a caravan!?) is behaving.

## Installation procedure


Add repository
```
helm repo add camel-dashboard https://camel-tooling.github.io/camel-dashboard/charts
```

Install chart
```
$ helm install camel-monitor-operator camel-dashboard/camel-monitor-operator -n camel
```

For more installation configuration on the Camel Monitor Operator please see the [installation documentation](https://camel-tooling.github.io/camel-dashboard/docs/installation-guide/operator/).
