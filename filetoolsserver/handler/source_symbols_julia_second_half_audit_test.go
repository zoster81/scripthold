package handler

import (
	"os"
	"testing"
)

// Provider 81 uses two further independent Julia projects, excluding the
// previously qualified compiler and Exercism projects. Selected originals
// were fully read and verified against immutable upstream Git blobs.
func TestSourceSymbolsJuliaSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Julia project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "julia81-statsbase-audit-selected", language: "julia",
			search: "_competerank!", primary: "src/ranking.jl",
			negativeFile: "src/reliability.jl", negativeQuery: "_competerank!",
			paths: []string{"src/ranking.jl", "src/reliability.jl"},
			oracles: []extraAuditEvidence{
				{"src/ranking.jl", "function _rank(f!, x::AbstractArray, R::Type=Int; sortkwargs...)"},
				{"src/ranking.jl", "function _rank(f!, x::AbstractArray{>: Missing}, R::Type=Int; sortkwargs...)"},
				{"src/ranking.jl", "function _competerank!(rks::AbstractArray, x::AbstractArray, p::AbstractArray{<:Integer})"},
				{"src/ranking.jl", "function _denserank!(rks::AbstractArray, x::AbstractArray, p::AbstractArray{<:Integer})"},
				{"src/ranking.jl", "function _tiedrank!(rks::AbstractArray, x::AbstractArray, p::AbstractArray{<:Integer})"},
				{"src/reliability.jl", "struct CronbachAlpha{T <: Real}"},
				{"src/reliability.jl", "function cronbachalpha(covmatrix::AbstractMatrix{<:Real})"},
			},
			targets: []extraAuditSymbol{
				{"src/ranking.jl", "_rank", 19},
				{"src/ranking.jl", "_rank", 26},
				{"src/ranking.jl", "_competerank!", 60},
				{"src/ranking.jl", "_denserank!", 97},
				{"src/ranking.jl", "_tiedrank!", 134},
				{"src/reliability.jl", "CronbachAlpha", 1},
				{"src/reliability.jl", "cronbachalpha", 53},
			},
		},
		{
			project: "julia81-csv-audit-selected", language: "julia",
			search: "_streampredicate", primary: "src/scan.jl",
			negativeFile: "src/tables.jl", negativeQuery: "_streampredicate",
			paths: []string{"src/scan.jl", "src/tables.jl"},
			oracles: []extraAuditEvidence{
				{"src/scan.jl", "struct PredicateColumns"},
				{"src/scan.jl", "function _executescanplan(src::IndexedSource, scan::Tables.Scan;"},
				{"src/scan.jl", "function _streampredicate(src::IndexedSource, plan::ColumnPlan, b::Tables.BoundScan,"},
				{"src/scan.jl", "function _reusepredicate(src::IndexedSource, plan::ColumnPlan, kept::Vector{Int},"},
				{"src/scan.jl", "function _cliprows!(mask::Vector{Bool}, offset::Int, limit::Union{Nothing, Int})"},
				{"src/tables.jl", "function settlebatchschema!(types::Vector{Type}, buf, chunks, plan::ColumnPlan,"},
				{"src/tables.jl", "struct Batches"},
				{"src/tables.jl", "function tryparsebatch(b::Batches, ci::ChunkIndex, n::Int,"},
				{"src/tables.jl", "function Base.iterate(r::_IndexedRows, state=(1, nothing, 1))"},
			},
			targets: []extraAuditSymbol{
				{"src/scan.jl", "PredicateColumns", 13},
				{"src/scan.jl", "_executescanplan", 61},
				{"src/scan.jl", "_streampredicate", 143},
				{"src/scan.jl", "_reusepredicate", 236},
				{"src/scan.jl", "_cliprows!", 294},
				{"src/scan.jl", "_project", 307},
				{"src/tables.jl", "settlebatchschema!", 34},
				{"src/tables.jl", "Batches", 164},
				{"src/tables.jl", "tryparsebatch", 179},
				{"src/tables.jl", "Base.iterate", 278},
				{"src/tables.jl", "Base.getindex", 303},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
