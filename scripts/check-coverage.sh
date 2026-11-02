#!/usr/bin/env bash
# Admit only the two expected reports before the token-bearing Sonar job.
set -euo pipefail

if [ -L coverage ] || [ ! -d coverage ]; then
  echo '::error::Missing or linked coverage directory'
  exit 1
fi

for report in coverage/coverage.out coverage/python-coverage.xml; do
  if [ -L "$report" ] || [ ! -f "$report" ] || [ ! -s "$report" ]; then
    echo '::error::Missing, empty or linked coverage report'
    exit 1
  fi
done

unexpected="$(find coverage -mindepth 1 ! -path coverage/coverage.out ! -path coverage/python-coverage.xml -print -quit)"
if [ -n "$unexpected" ]; then
  echo '::error::Unexpected entries in the coverage directory'
  exit 1
fi

if grep -Eq '<!DOCTYPE|<!ENTITY' coverage/python-coverage.xml; then
  echo '::error::XML declarations for document types or entities are not allowed'
  exit 1
fi
