package handler

import "testing"

func TestParseKnowledgeImportFileCSV(t *testing.T) {
	articles, err := parseKnowledgeImportFile("products.csv", []byte("\ufefftitle,content,category,attributes\n夏装,透气舒适,服饰,\"{\"\"price\"\":99}\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].Title != "夏装" || articles[0].Attributes != `{"price":99}` {
		t.Fatalf("articles = %#v", articles)
	}
}

func TestParseKnowledgeImportFileCSVAllowsOnlyRequiredColumns(t *testing.T) {
	articles, err := parseKnowledgeImportFile("faq.csv", []byte("title,content\n退货政策,签收后七天可退货\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].Category != "" || articles[0].Attributes != "{}" {
		t.Fatalf("articles = %#v", articles)
	}
}

func TestParseKnowledgeImportFileJSONAcceptsObjectAttributes(t *testing.T) {
	articles, err := parseKnowledgeImportFile("faq.json", []byte(`[{"title":"运费","content":"满 99 元包邮","attributes":{"threshold":99}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].Attributes != `{"threshold":99}` {
		t.Fatalf("articles = %#v", articles)
	}
}

func TestParseKnowledgeImportFileRejectsInvalidRows(t *testing.T) {
	if _, err := parseKnowledgeImportFile("faq.csv", []byte("title,content\n只有标题,\n")); err == nil {
		t.Fatal("expected invalid CSV row to fail")
	}
	if _, err := parseKnowledgeImportFile("faq.pdf", []byte("not supported")); err == nil {
		t.Fatal("expected unsupported extension to fail")
	}
}
