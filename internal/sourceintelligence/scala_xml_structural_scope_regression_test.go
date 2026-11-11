package sourceintelligence

import (
	"context"
	"testing"
)

func TestScalaXMLClassAndMethodScopeDoesNotInventLocalFields(t *testing.T) {
	source := `package scala
package xml
object Utility {
  private[xml] def sbToString(f: StringBuilder => Unit): String = {
    val sb: StringBuilder = new StringBuilder()
    f(sb)
    sb.toString
  }
  def sequenceToXML(
    children: Seq[String]
  ): Unit = if (children.nonEmpty) {
    val spaced = children.forall(_ != "")
    println(spaced)
  }
}
class Elem(
  override val prefix: String,
  override val label: String,
  attributes1: String,
  override val scope: String,
  val minimizeEmpty: Boolean,
  override val child: Seq[String]
) extends Node with Serializable {
  final override def doCollectNamespaces: Boolean = true
  override val attributes: String = attributes1
  final def %(updates: String): Elem = this
  def copy(prefix: String = this.prefix): Elem = this
  override def text: String = child.mkString
}
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unexpected incomplete structural source: %+v", result.Analysis)
	}
	expected := map[string]SymbolKind{
		"scala":                              SymbolKindPackage,
		"scala.xml":                          SymbolKindPackage,
		"scala.xml.Utility":                  SymbolKindModule,
		"scala.xml.Utility.sbToString":       SymbolKindMethod,
		"scala.xml.Utility.sequenceToXML":    SymbolKindMethod,
		"scala.xml.Elem":                     SymbolKindClass,
		"scala.xml.Elem.prefix":              SymbolKindField,
		"scala.xml.Elem.label":               SymbolKindField,
		"scala.xml.Elem.scope":               SymbolKindField,
		"scala.xml.Elem.minimizeEmpty":       SymbolKindField,
		"scala.xml.Elem.child":               SymbolKindField,
		"scala.xml.Elem.doCollectNamespaces": SymbolKindMethod,
		"scala.xml.Elem.attributes":          SymbolKindField,
		"scala.xml.Elem.%":                   SymbolKindMethod,
		"scala.xml.Elem.copy":                SymbolKindMethod,
		"scala.xml.Elem.text":                SymbolKindMethod,
	}
	actual := symbolsByQualifiedName(result.Analysis.Symbols)
	if len(result.Analysis.Symbols) != len(expected) {
		t.Errorf("expected %d declarations, received %d: %v", len(expected), len(result.Analysis.Symbols), sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	for name, kind := range expected {
		v, exists := actual[name]
		if !exists || v.Kind != kind {
			t.Errorf("expected %s %s, got %+v (present=%v)", kind, name, v, exists)
		}
	}
	for name := range actual {
		if _, exists := expected[name]; !exists {
			t.Errorf("invented or improperly owned Scala declaration %s", name)
		}
	}
}
