# Troubleshooting

- Auth denied:
  - Verify token issuance and actor metadata alignment.
- Transport failures:
  - Confirm gRPC-Web proxy or REST endpoint reachability.
- Financial request denied:
  - Ensure idempotency key is set for state-changing operations.
