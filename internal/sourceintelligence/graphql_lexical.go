package sourceintelligence

func maskGraphQLSource(text string) string {
	masked := maskSourceStrings(text, false, true, true)
	return maskSourceComments(masked, []string{"#"}, "", "")
}
