#!/bin/sh
# Check that the running services report ready.
curl -fsS http://localhost:8080/v1/ready || exit 1
curl -fsS http://localhost:8081/v1/ready || exit 1
curl -fsS http://localhost:8000/health || exit 1
