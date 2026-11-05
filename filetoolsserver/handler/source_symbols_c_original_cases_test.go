package handler

func originalCScenarios() []originalCScenario {
	return []originalCScenario{
		{project: "c-davegamble-cjson", query: "parse_array", unrelated: "tests/print_number.c",
			files:   []string{"cJSON.h", "cJSON.c", "tests/parse_array.c", "tests/parse_object.c", "tests/print_number.c"},
			oracle:  []originalCSource{{"cJSON.h", "#define CJSON_NESTING_LIMIT 1000"}, {"cJSON.c", "if (input_buffer->depth >= CJSON_NESTING_LIMIT)"}, {"tests/parse_array.c", "parse_array_should_parse_arrays_with_multiple_elements"}, {"tests/parse_object.c", "parse_object_should_parse_objects_with_multiple_elements"}},
			targets: []originalCTarget{{"cJSON.c", "parse_array", "input_buffer->depth >= CJSON_NESTING_LIMIT", 1497}, {"cJSON.c", "parse_object", "input_buffer->depth >= CJSON_NESTING_LIMIT", 1662}, {"cJSON.c", "parse_value", "return parse_array(item, input_buffer);", 1368}, {"tests/parse_array.c", "parse_array_should_parse_arrays_with_multiple_elements", "assert_parse_array", 104}, {"tests/parse_object.c", "parse_object_should_parse_objects_with_multiple_elements", "assert_parse_object", 107}},
		},
		{project: "c-madler-zlib", query: "deflateInit2_", unrelated: "inflate.c",
			files:   []string{"zlib.h", "deflate.c", "test/example.c", "inflate.c"},
			oracle:  []originalCSource{{"zlib.h", "#define Z_DEFAULT_COMPRESSION  (-1)"}, {"deflate.c", "if (level == Z_DEFAULT_COMPRESSION) level = 6;"}, {"test/example.c", "err = deflateInit(&c_stream, Z_DEFAULT_COMPRESSION);"}},
			targets: []originalCTarget{{"deflate.c", "deflateInit_", "return deflateInit2_(strm, level", 379}, {"deflate.c", "deflateInit2_", "if (level == Z_DEFAULT_COMPRESSION) level = 6;", 387}, {"deflate.c", "deflateParams", "if (level == Z_DEFAULT_COMPRESSION) level = 6;", 774}, {"test/example.c", "test_deflate", "err = deflateInit(&c_stream, Z_DEFAULT_COMPRESSION);", 172}},
		},
	}
}
