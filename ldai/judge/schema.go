package judge

func buildSchema(metricKey string) map[string]interface{} {
	if metricKey == "" {
		return map[string]interface{}{}
	}

	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"evaluations": map[string]interface{}{
				"type":        "object",
				"description": "Object containing evaluation results for " + metricKey + " metric",
				"properties": map[string]interface{}{
					metricKey: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"score": map[string]interface{}{
								"type":        "number",
								"minimum":     0.0,
								"maximum":     1.0,
								"description": "Score between 0.0 and 1.0 for " + metricKey,
							},
							"reasoning": map[string]interface{}{
								"type":        "string",
								"description": "Reasoning behind the score for " + metricKey,
							},
						},
						"required":             []string{"score", "reasoning"},
						"additionalProperties": false,
					},
				},
				"required":             []string{metricKey},
				"additionalProperties": false,
			},
		},
		"required":             []string{"evaluations"},
		"additionalProperties": false,
	}
}
