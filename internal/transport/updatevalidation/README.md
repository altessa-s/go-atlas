# updatevalidation

The gRPC mask interceptors resolve a registered update spec once and share the
selection with the request validator. The policy cache classifies fields by
descriptor using the spec's resolver, identity fields, and immutable fields.
Mask admissibility and atomic path rules are applied during resolution.

For partial map validation, exact keys and wildcard keys contribute to the
same field scope. A whole-entry selection covers all descendants. The
validator applies the scope to a private copy when it needs to prune a map.
