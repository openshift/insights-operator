#!/bin/bash
# should be run from project root directory
for file in ./tests/integration/resource_samples/*.yaml
do
  oc apply -f $file
done