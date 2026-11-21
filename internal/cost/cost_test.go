package cost_test

import (
	"math"
	"strings"
	"testing"

	"dioptra/internal/contract"
	"dioptra/internal/cost"
)

func TestEstimateTokens_DeterminismAndMonotonicity(t *testing.T) {
	short := "Hola mundo."
	long := strings.Repeat("# Sección de auditoría\nLatencia p95: 340 ms en internal/store/store.go:622.\n", 20)
	tShort := cost.EstimateTokens(short)
	tLong := cost.EstimateTokens(long)
	if tShort <= 0 || tLong <= tShort*10 {
		t.Fatalf("conteo de tokens inconsistente: short=%d long=%d", tShort, tLong)
	}
	if cost.EstimateTokens("") != 0 {
		t.Fatalf("cadena vacía debe devolver 0 tokens")
	}
}

func TestCompute_GoldmarkVsBlocksOverrideAndInvariant(t *testing.T) {
	md := strings.Repeat("## Hallazgo técnico\n- Latencia: 340 ms en `internal/store/store.go:622`.\n- Riesgo de saturación en cola.\n\n", 10)
	blocksJSON := []byte(`[{"t":"kpi","items":[{"label":"p95","value":340,"unit":"ms"}]},{"t":"prose","md":"Resumen completo del hallazgo técnico repetido con detalle adicional y trazas estructuradas."}]`)

	fast := cost.Compute(cost.ComputeParams{
		Mode:     "fast_goldmark",
		Model:    "claude-sonnet-4-5",
		Markdown: md,
	})
	rich := cost.Compute(cost.ComputeParams{
		Mode:              "blocks_override",
		Model:             "claude-sonnet-4-5",
		Markdown:          md,
		BlocksOverrideRaw: blocksJSON,
	})

	if fast.IsExact || rich.IsExact {
		t.Fatalf("sin LayoutMeta los costos deben marcarse como estimados")
	}
	if fast.ExtraOutputTokens <= 0 || fast.DeltaPct >= 40 {
		t.Fatalf("fast_goldmark debe tener sobrecarga pequeña, obtuvo: %+v", fast)
	}
	if rich.ExtraOutputTokens <= fast.ExtraOutputTokens || rich.DeltaUSD <= fast.DeltaUSD {
		t.Fatalf("blocks_override debe medir mayor aumento de tokens y USD que fast_goldmark: fast=%+v rich=%+v", fast, rich)
	}
	if rich.ContractNote == "" || !strings.Contains(rich.ConsoleLine, "USD") {
		t.Fatalf("ConsoleLine / ContractNote incompletos: %+v", rich)
	}
	if diff := math.Abs(rich.DeltaUSD - (rich.DioptraUSD - rich.BaselineUSD)); diff > 1e-6 {
		t.Fatalf("invariante DeltaUSD rota: delta=%.6f diff=%.6f", rich.DeltaUSD, diff)
	}
}

func TestCompute_LayoutAgentExactTokensAndEnvPricing(t *testing.T) {
	t.Setenv("DIOPTRA_COST_INPUT_PER_1M", "2.00")
	t.Setenv("DIOPTRA_COST_OUTPUT_PER_1M", "10.00")

	md := "# Reporte\n\nTexto técnico con suficiente longitud para comparar consumo real vs consola.\n"
	rep := cost.Compute(cost.ComputeParams{
		Mode:     "layout_agent",
		Markdown: md,
		LayoutMeta: &contract.LayoutMeta{
			Provider:     "anthropic",
			Model:        "custom-model",
			InputTokens:  3000,
			OutputTokens: 1000,
		},
	})
	if !rep.IsExact || !strings.Contains(rep.ConsoleLine, "real") || !strings.Contains(rep.PricingLabel, "$2.00/$10.00") {
		t.Fatalf("no usó métricas reales ni tarifa env: %+v", rep)
	}
	if diff := math.Abs(rep.DeltaUSD - (rep.DioptraUSD - rep.BaselineUSD)); diff > 1e-6 {
		t.Fatalf("invariante DeltaUSD rota: %+v", rep)
	}
}

func TestResolvePricing_ModelCatalogAndLocalZero(t *testing.T) {
	_, opus := cost.ResolvePricing("claude-opus-4-1")
	_, flash := cost.ResolvePricing("gemini-2.5-flash")
	_, cli := cost.ResolvePricing("cli")
	if opus.OutputPer1M <= flash.OutputPer1M {
		t.Fatalf("tarifa opus debe superar flash: opus=%+v flash=%+v", opus, flash)
	}
	if cli.InputPer1M != 0 || cli.OutputPer1M != 0 {
		t.Fatalf("proveedor cli local debe tener tarifa base 0: %+v", cli)
	}
}

func TestResolvePricing_VerifiedGatewayRatesAndSource(t *testing.T) {
	t.Setenv("DIOPTRA_COST_INPUT_PER_1M", "")
	t.Setenv("DIOPTRA_COST_OUTPUT_PER_1M", "")
	cases := []struct {
		model      string
		in, out    float64
		wantSource string
	}{
		{"claude-opus-5-5", 4, 20, cost.SourceVerified},
		{"claude-sonnet-5-5", 2, 10, cost.SourceVerified},
		// Las reglas de familia genéricas NO deben quedar atrapadas por las verificadas.
		{"claude-opus-4-6", 15, 75, cost.SourceReference},
		{"claude-sonnet-4-5", 3, 15, cost.SourceReference},
	}
	for _, tc := range cases {
		_, p := cost.ResolvePricing(tc.model)
		if p.InputPer1M != tc.in || p.OutputPer1M != tc.out || p.Source != tc.wantSource {
			t.Errorf("%s: got in=%v out=%v source=%q, want in=%v out=%v source=%q",
				tc.model, p.InputPer1M, p.OutputPer1M, p.Source, tc.in, tc.out, tc.wantSource)
		}
	}
}

func TestResolvePricing_EnvOverridesCatalogAndMarksSource(t *testing.T) {
	t.Setenv("DIOPTRA_COST_INPUT_PER_1M", "1")
	t.Setenv("DIOPTRA_COST_OUTPUT_PER_1M", "5")
	_, p := cost.ResolvePricing("claude-opus-5-5")
	if p.InputPer1M != 1 || p.OutputPer1M != 5 || p.Source != cost.SourceEnv {
		t.Fatalf("la variable de entorno debe ganar al catálogo y marcarse como env: %+v", p)
	}
}

func TestCompute_ExposesPricingSource(t *testing.T) {
	t.Setenv("DIOPTRA_COST_INPUT_PER_1M", "")
	t.Setenv("DIOPTRA_COST_OUTPUT_PER_1M", "")
	rep := cost.Compute(cost.ComputeParams{Mode: "fast_goldmark", Model: "claude-opus-5-5", Markdown: "## Hola\n- uno\n"})
	if rep.PricingSource != cost.SourceVerified || !strings.Contains(rep.ConsoleLine, "tarifa verified") {
		t.Fatalf("el reporte debe indicar la procedencia de la tarifa: %+v", rep)
	}
}
