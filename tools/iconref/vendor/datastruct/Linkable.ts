// VENDORED VERBATIM from LostCityRS/Client-TS (branch 274-GOSCAPE, commit b67894260fb06ae6162ed3a8adab506abcd7faa9)
// source: src/datastruct/Linkable.ts  blobSHA: c2de87fc5d6645ab432f3a697dc49a23689b3373
// Only this provenance header was added; file body is unmodified.
export default class Linkable {
    key: bigint = 0n;
    next: Linkable | null = null;
    prev: Linkable | null = null;

    unlink(): void {
        if (this.prev != null) {
            this.prev.next = this.next;
            if (this.next) {
                this.next.prev = this.prev;
            }
            this.next = null;
            this.prev = null;
        }
    }
}
