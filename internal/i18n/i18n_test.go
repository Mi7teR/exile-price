package i18n

import "testing"

func TestCount(t *testing.T) {
	defer Set(EN)
	Set(EN)
	if got := Count(1, "listing") + "," + Count(7, "listing"); got != "1 listing,7 listings" {
		t.Errorf("en: %q", got)
	}
	Set(RU)
	for n, want := range map[int]string{1: "1 лот", 3: "3 лота", 7: "7 лотов", 11: "11 лотов", 21: "21 лот", 22: "22 лота", 112: "112 лотов"} {
		if got := Count(n, "listing"); got != want {
			t.Errorf("ru %d: %q, want %q", n, got, want)
		}
	}
}

func TestT(t *testing.T) {
	defer Set(EN)
	Set(RU)
	if T("Save") != "Сохранить" || T("no such key") != "no such key" {
		t.Error("ru lookup or fallback broken")
	}
	if Tf("done · league %s", "Covens") != "готово · лига Covens" {
		t.Error("Tf")
	}
	if Parse("ru") != RU || Parse("RU") != RU || Parse("en") != EN || Parse("") != EN || Parse("de") != EN {
		t.Error("Parse")
	}
}

func TestSetWhileReading(t *testing.T) { // run with -race: the settings switch the language live
	defer Set(EN)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			Set(Lang(i % 2))
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_ = T("Save") + Count(i, "listing")
	}
	<-done
}
