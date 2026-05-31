package zone

import "iter"

// Element is an intrusive doubly-linked-list node owning a Value of type T.
// Stored as *Element by callers so they can call Unlink() in O(1).
//
// Mirrors TS DoublyLinkable's role in Engine-TS's #/datastruct/DoublyLinkList,
// translated to Element-based composition (Go doesn't support TS's abstract-
// base inheritance shape; behavior is identical — same O(1) cost, same
// iteration order, same visible state).
type Element[T any] struct {
	next, prev *Element[T]
	list       *DoublyLinkList[T]
	Value      T
}

// Unlink removes e from its list. Idempotent — second call is a no-op
// (mirrors TS DoublyLinkable.unlink2 idempotency).
func (e *Element[T]) Unlink() {
	if e.list == nil {
		return
	}
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		e.list.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		e.list.tail = e.prev
	}
	e.list.size--
	e.next, e.prev, e.list = nil, nil, nil
}

// DoublyLinkList is an intrusive doubly-linked list. Zero value is a valid
// empty list. All operations are O(1) except All which is O(N).
//
// Mirrors TS DoublyLinkList<T> at Engine-TS/datastruct/DoublyLinkList.ts.
type DoublyLinkList[T any] struct {
	head, tail *Element[T]
	size       int
}

// AddTail appends v to the end of the list and returns the new Element.
// Caller stores the *Element to support O(1) Unlink.
func (l *DoublyLinkList[T]) AddTail(v T) *Element[T] {
	e := &Element[T]{Value: v, list: l, prev: l.tail}
	if l.tail != nil {
		l.tail.next = e
	} else {
		l.head = e
	}
	l.tail = e
	l.size++
	return e
}

// Size returns the number of elements in the list.
func (l *DoublyLinkList[T]) Size() int { return l.size }

// All returns an iterator over the list's values. reverse=false yields
// in insertion order; reverse=true yields in reverse insertion order.
//
// Safe under mid-iteration removal of the just-yielded element: the
// next/prev pointer is captured BEFORE yield, so a yield body that
// calls Unlink() on the current node — which clears that node's
// next/prev to nil — does not strand the iterator. Mirrors TS
// DoublyLinkList.all (DoublyLinkList.ts:73-87), which saves
// `this.cursor` before each yield and restores it after, exactly to
// survive in-flight removal. Removing OTHER nodes during a yield
// (specifically the saved next/prev) is still unsafe — TS shares the
// same limitation. 2026-05-28 audit row datastruct-db-1.
func (l *DoublyLinkList[T]) All(reverse bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		if reverse {
			for n := l.tail; n != nil; {
				save := n.prev
				if !yield(n.Value) {
					return
				}
				n = save
			}
			return
		}
		for n := l.head; n != nil; {
			save := n.next
			if !yield(n.Value) {
				return
			}
			n = save
		}
	}
}
