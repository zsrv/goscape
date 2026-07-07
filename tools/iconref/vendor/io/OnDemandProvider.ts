// VENDORED VERBATIM from LostCityRS/Client-TS (branch 274-GOSCAPE, commit b67894260fb06ae6162ed3a8adab506abcd7faa9)
// source: src/io/OnDemandProvider.ts  blobSHA: 55915f18f94a0d5fbf5db8653542ffb4bacf926b
// Only this provenance header was added; file body is unmodified.
export default abstract class OnDemandProvider {
    abstract requestModel(id: number): void;
}
