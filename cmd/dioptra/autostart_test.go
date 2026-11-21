package main

import "testing"

func TestAutostartCommandLineQuotesPathsWithSpaces(t *testing.T) {
	got := autostartCommandLine(`C:\Windows\System32\conhost.exe`,
		`C:\Users\dev\My Apps\dioptra.exe`, 7474, `C:\Users\dev\.dioptra\reports`)
	want := `"C:\Windows\System32\conhost.exe" --headless "C:\Users\dev\My Apps\dioptra.exe" start --port 7474 --dir "C:\Users\dev\.dioptra\reports"`
	if got != want {
		t.Fatalf("línea de arranque:\n got  %s\n want %s", got, want)
	}
	if exe := autostartExecutable(got); exe != `C:\Users\dev\My Apps\dioptra.exe` {
		t.Fatalf("autostartExecutable = %q", exe)
	}
}

func TestQuoteWinArgTrailingBackslash(t *testing.T) {
	// Sin duplicar la barra final, `"C:\dir\"` se leería como comilla escapada.
	if got := quoteWinArg(`C:\dir\`); got != `"C:\dir\\"` {
		t.Fatalf("quoteWinArg = %s", got)
	}
}
