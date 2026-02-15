# Troubleshooting

- Auth denied:
  - Verify token issuance and actor metadata alignment.
- Transport failures:
  - Confirm gRPC-Web proxy or REST endpoint reachability.
  - For WebGL runtime builds, use `RestGateway` transport mode.
  - Ensure API CORS policy allows browser origin, methods, and headers (including `authorization`).
- Financial request denied:
  - Ensure idempotency key is set for state-changing operations.
