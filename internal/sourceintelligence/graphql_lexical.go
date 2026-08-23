package sourceintelligence

func maskGraphQLSource(text string) string {
	masked := phase10MaskStrings(text, false, true, true)
	return phase10MaskComments(masked, []string{"#"}, "", "")
}
