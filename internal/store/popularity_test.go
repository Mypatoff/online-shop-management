package store

import "testing"

func TestListProductsByPopularity(t *testing.T) {
	s := newTestStore(t)
	apple := mustCreateProduct(t, s, "Apple", "", 100, 50, 1)
	bread := mustCreateProduct(t, s, "Bread", "", 100, 50, 1)
	cola := mustCreateProduct(t, s, "Cola", "", 100, 50, 1)
	mustCreateProduct(t, s, "Dates", "", 100, 50, 1) // never sold

	if _, _, err := s.CreateSale(bread.ID, 2, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSale(cola.ID, 5, 0); err != nil {
		t.Fatal(err)
	}
	// A voided sale must not count: Apple would otherwise lead with 9.
	voided, _, err := s.CreateSale(apple.ID, 9, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VoidSale(voided.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListProductsByPopularity("", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Cola", "Bread", "Apple", "Dates"} // unsold ones keep A-Z order
	if len(got) != len(want) {
		t.Fatalf("got %d products, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d = %s, want %s", i, got[i].Name, name)
		}
	}

	// The plain listing is still alphabetical.
	plain, err := s.ListProducts("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain[0].Name != "Apple" || plain[3].Name != "Dates" {
		t.Errorf("ListProducts no longer alphabetical: %v", plain)
	}
}
