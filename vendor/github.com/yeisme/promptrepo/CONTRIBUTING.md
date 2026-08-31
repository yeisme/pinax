# Contributing

感谢贡献。请保持变更小而聚焦，并保留已发布合同的兼容性。

Thanks for contributing. Keep changes small and focused, and preserve released
contracts.

1. Discuss contract, state-schema, or public API changes in an OpenSpec change
   before implementation.
2. Do not add Template Registry server/internal dependencies or credentials.
3. Keep state schema names, digests, error codes, and exact-ref behavior
   backward compatible; add a migration, deprecation window, and rollback plan
   for any exception.
4. Run the checks in the README, including the CGO-disabled test path.
5. Add or update public compatibility tests for behavioral changes.

Do not commit generated state, caches, test artifacts, private catalogs, or
prompt bodies that are not intended for public distribution.
