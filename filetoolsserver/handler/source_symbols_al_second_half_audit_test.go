package handler

import (
	"os"
	"testing"
)

// Provider 69: Microsoft's BCTech samples and AL extension examples are
// independent project origins, not the initially qualified PowerSearch and
// Sticky Notes repositories. Every selected source and neighboring test was
// fully read and pinned to its immutable upstream blob before analyzer queries.
func TestSourceSymbolsALSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original AL projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "al69-bctech-audit-selected", language: "al",
			search:        "GenerateItemProposal",
			primary:       "samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al",
			negativeFile:  "samples/AzureOpenAI/2-ItemSubstitution/Test/SelfSubstitutionTest.Codeunit.al",
			negativeQuery: "GenerateItemProposal",
			paths: []string{
				"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al",
				"samples/AzureOpenAI/2-ItemSubstitution/Test/ItemSubstitutionTest.Codeunit.al",
				"samples/AzureOpenAI/2-ItemSubstitution/Test/SelfSubstitutionTest.Codeunit.al",
			},
			oracles: []extraAuditEvidence{
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "codeunit 54323 \"Generate Item Sub Proposal\""},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "trigger OnRun()"},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "local procedure GenerateItemProposal()"},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "TmpText := Chat(GetSystemPrompt(), GetFinalUserPrompt(UserPrompt));"},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "AOAIChatCompletionParams.SetJsonMode(true);"},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "JsonItemsArray := JResItemsTok.AsArray();"},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "if SuggestOnlyAvailableItems then"},
				{"samples/AzureOpenAI/2-ItemSubstitution/Test/ItemSubstitutionTest.Codeunit.al", "procedure TestSubstitutionAvailableItems()"},
				{"samples/AzureOpenAI/2-ItemSubstitution/Test/ItemSubstitutionTest.Codeunit.al", "GenerateItemSubProposal.SetSuggestOnlyAvailableItems();"},
				{"samples/AzureOpenAI/2-ItemSubstitution/Test/SelfSubstitutionTest.Codeunit.al", "procedure TestNoSelfSubstitution()"},
				{"samples/AzureOpenAI/2-ItemSubstitution/Test/SelfSubstitutionTest.Codeunit.al", "if TmpItemSubstAIProposal.\"No.\" = SubstitutedItemNo then"},
			},
			targets: []extraAuditSymbol{
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "SetUserPrompt", 16},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "GetResult", 21},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "SetSuggestOnlyAvailableItems", 26},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "GetCompletionResult", 31},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "GenerateItemProposal", 36},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "Chat", 87},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "GetFinalUserPrompt", 139},
				{"samples/AzureOpenAI/2-ItemSubstitution/CopilotCodeunits/GenerateItemSubProposal.Codeunit.al", "GetSystemPrompt", 160},
				{"samples/AzureOpenAI/2-ItemSubstitution/Test/ItemSubstitutionTest.Codeunit.al", "TestItemSubWorking", 13},
				{"samples/AzureOpenAI/2-ItemSubstitution/Test/ItemSubstitutionTest.Codeunit.al", "TestSubstitutionAvailableItems", 51},
				{"samples/AzureOpenAI/2-ItemSubstitution/Test/SelfSubstitutionTest.Codeunit.al", "TestNoSelfSubstitution", 13},
			},
		},
		{
			project: "al69-vscode-samples-audit-selected", language: "al",
			search:        "ConnectToServiceAndFillBuffer",
			primary:       "samples/GetAddressService/Codeunits/AddressService.al",
			negativeFile:  "samples/GetAddressService/Tables/ServiceConnectionSetup.al",
			negativeQuery: "ConnectToServiceAndFillBuffer",
			paths: []string{
				"samples/GetAddressService/Codeunits/AddressService.al",
				"samples/GetAddressService/Pages/Addresses.al",
				"samples/GetAddressService/Tables/ServiceConnectionSetup.al",
			},
			oracles: []extraAuditEvidence{
				{"samples/GetAddressService/Codeunits/AddressService.al", "codeunit 50102 \"Address Service\""},
				{"samples/GetAddressService/Codeunits/AddressService.al", "CreateAddressLookupnotification(Rec);"},
				{"samples/GetAddressService/Codeunits/AddressService.al", "dataValid := dataValid and typeHelper.IsMatch(postCode,PostCodeValidationRegExTxt);"},
				{"samples/GetAddressService/Codeunits/AddressService.al", "procedure Lookupaddresses(addressNotification : Notification);"},
				{"samples/GetAddressService/Codeunits/AddressService.al", "Getaddresses(addressNotification.GetData('postCode'),tempAddress);"},
				{"samples/GetAddressService/Codeunits/AddressService.al", "if (not httpResponse.IsSuccessStatusCode) then"},
				{"samples/GetAddressService/Codeunits/AddressService.al", "jsonResponse.SelectToken('Addresses', addressesToken)"},
				{"samples/GetAddressService/Codeunits/AddressService.al", "UpdateRecordWithAddressInformation(recRef,tempAddress);"},
				{"samples/GetAddressService/Pages/Addresses.al", "procedure SetRecords(var tempAddress : Record Address temporary);"},
				{"samples/GetAddressService/Pages/Addresses.al", "SourceTableTemporary = True;"},
				{"samples/GetAddressService/Tables/ServiceConnectionSetup.al", "field(3;\"API Key\";Text[100])"},
				{"samples/GetAddressService/Tables/ServiceConnectionSetup.al", "ExtendedDataType = Masked;"},
			},
			targets: []extraAuditSymbol{
				{"samples/GetAddressService/Codeunits/AddressService.al", "OnCustomerpostCodeValidate", 15},
				{"samples/GetAddressService/Codeunits/AddressService.al", "OnVendorpostCodeValidate", 21},
				{"samples/GetAddressService/Codeunits/AddressService.al", "HandleAddressLookupServiceConnection", 27},
				{"samples/GetAddressService/Codeunits/AddressService.al", "CreateAddressLookupnotification", 49},
				{"samples/GetAddressService/Codeunits/AddressService.al", "postCodeValid", 76},
				{"samples/GetAddressService/Codeunits/AddressService.al", "Lookupaddresses", 96},
				{"samples/GetAddressService/Codeunits/AddressService.al", "Getaddresses", 123},
				{"samples/GetAddressService/Codeunits/AddressService.al", "GetAddressFromAddressString", 138},
				{"samples/GetAddressService/Codeunits/AddressService.al", "ConnectToServiceAndFillBuffer", 151},
				{"samples/GetAddressService/Codeunits/AddressService.al", "UpdateRecordWithAddressInformation", 188},
				{"samples/GetAddressService/Codeunits/AddressService.al", "AppendToString", 203},
				{"samples/GetAddressService/Pages/Addresses.al", "SetRecords", 48},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
