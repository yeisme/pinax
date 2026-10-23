package schema

const ReaderRecoveryLookupRequestV1Alpha1 = "pinax.reader.recovery_lookup.request.v1alpha1"

func readerRecoveryDefinitions() []Definition {
	return []Definition{define(ReaderRecoveryLookupRequestV1Alpha1, closedObject(map[string]any{
		"operation_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[A-Za-z0-9][A-Za-z0-9._:-]*$"},
	}, "operation_id"))}
}
