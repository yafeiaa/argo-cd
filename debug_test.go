package cache

import (
"fmt"
"testing"
)

func TestDebugYAML(t *testing.T) {
	manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 5
status:
  replicas: 5
  updatedReplicas: 3
  readyReplicas: 3
  availableReplicas: 3
`
	
	un := strToUnstructured(manifest)
	fmt.Printf("Parsed object: %+v\n", un.Object)
	
	replicas, found, err := unstructured.NestedInt64(un.Object, "spec", "replicas")
	fmt.Printf("Replicas: %d, found: %v, err: %v\n", replicas, found, err)
	
	updatedReplicas, _, _ := unstructured.NestedInt64(un.Object, "status", "updatedReplicas")
	fmt.Printf("Updated replicas: %d\n", updatedReplicas)
}
