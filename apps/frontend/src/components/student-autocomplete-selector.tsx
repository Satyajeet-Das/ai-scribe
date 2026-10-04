"use client";

import React, { useState, useEffect, useRef, useId, useCallback } from "react";
import { Search, Loader2, Check, UserCheck, X, AlertCircle } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useDebounce } from "@/hooks/use-debounce";
import { studentsApi } from "@/services/api";
import type { StudentSearchResult } from "@/types/auth";

export interface StudentAutocompleteSelectorProps {
  onSelect: (student: StudentSearchResult | null) => void;
  selectedStudent: StudentSearchResult | null;
  existingStudentIds?: string[];
  placeholder?: string;
  minChars?: number;
  disabled?: boolean;
}

export function StudentAutocompleteSelector({
  onSelect,
  selectedStudent,
  existingStudentIds = [],
  placeholder = "Start typing roll number (e.g. 23CS001) or student name...",
  minChars = 2,
  disabled = false,
}: StudentAutocompleteSelectorProps) {
  const [searchTerm, setSearchTerm] = useState("");
  const [results, setResults] = useState<StudentSearchResult[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState<number>(-1);
  const [searchError, setSearchError] = useState<string | null>(null);

  const debouncedQuery = useDebounce(searchTerm.trim(), 300);
  const abortControllerRef = useRef<AbortController | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const listboxRef = useRef<HTMLUListElement>(null);

  const componentId = useId();
  const inputId = `student-search-input-${componentId}`;
  const listboxId = `student-search-listbox-${componentId}`;

  // Perform search with cancellation of previous in-flight requests
  useEffect(() => {
    if (debouncedQuery.length < minChars) {
      return;
    }

    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    const controller = new AbortController();
    abortControllerRef.current = controller;

    let isMounted = true;
    studentsApi
      .searchStudents(debouncedQuery, 10, controller.signal)
      .then((data) => {
        if (isMounted && !controller.signal.aborted) {
          setResults(data);
          setIsLoading(false);
          setIsOpen(true);
          setHighlightedIndex(-1);
          setSearchError(null);
        }
      })
      .catch((err: unknown) => {
        if (isMounted && !controller.signal.aborted) {
          setIsLoading(false);
          // Only show error if not an abort
          if (err instanceof Error && err.name !== "AbortError") {
            setSearchError("Failed to fetch students. Please try again.");
          }
        }
      });

    return () => {
      isMounted = false;
      controller.abort();
    };
  }, [debouncedQuery, minChars]);

  const handleSelectStudent = useCallback(
    (student: StudentSearchResult) => {
      const isAlreadyAssigned = existingStudentIds.includes(student.id);
      if (isAlreadyAssigned) return;

      onSelect(student);
      setIsOpen(false);
      setSearchTerm("");
      setResults([]);
    },
    [existingStudentIds, onSelect]
  );

  const handleClearSelection = () => {
    onSelect(null);
    setSearchTerm("");
    setResults([]);
    setIsOpen(false);
    setTimeout(() => {
      inputRef.current?.focus();
    }, 0);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!isOpen && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
      if (results.length > 0) {
        setIsOpen(true);
        e.preventDefault();
      }
      return;
    }

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightedIndex((prev) => (prev < results.length - 1 ? prev + 1 : 0));
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightedIndex((prev) => (prev > 0 ? prev - 1 : results.length - 1));
        break;
      case "Enter":
        e.preventDefault();
        if (highlightedIndex >= 0 && highlightedIndex < results.length) {
          handleSelectStudent(results[highlightedIndex]);
        }
        break;
      case "Escape":
        e.preventDefault();
        setIsOpen(false);
        setHighlightedIndex(-1);
        break;
      default:
        break;
    }
  };

  // Scroll highlighted item into view
  useEffect(() => {
    if (highlightedIndex >= 0 && listboxRef.current) {
      const optionEl = listboxRef.current.children[highlightedIndex] as HTMLElement;
      if (typeof optionEl?.scrollIntoView === "function") {
        optionEl.scrollIntoView({ block: "nearest" });
      }
    }
  }, [highlightedIndex]);

  return (
    <div className="relative w-full space-y-2">
      {/* Selected Student Display */}
      {selectedStudent ? (
        <div
          className="flex items-center justify-between rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm transition-all"
          data-testid="selected-student-card"
        >
          <div className="flex items-center gap-3 overflow-hidden">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
              <UserCheck className="size-5" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-mono text-xs font-bold text-foreground">
                  {selectedStudent.rollNo || selectedStudent.roll_no}
                </span>
                <span className="text-muted-foreground">—</span>
                <span className="font-medium text-foreground truncate">
                  {selectedStudent.name}
                </span>
              </div>
              <p className="text-xs text-muted-foreground truncate">{selectedStudent.email}</p>
            </div>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={handleClearSelection}
            disabled={disabled}
            className="h-8 px-2 text-xs text-muted-foreground hover:text-foreground"
            aria-label={`Change selected student ${selectedStudent.name}`}
          >
            <X className="mr-1 size-3.5" />
            Change
          </Button>
        </div>
      ) : (
        /* Autocomplete Combobox Input */
        <div className="relative">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground pointer-events-none" />
            <Input
              ref={inputRef}
              id={inputId}
              role="combobox"
              aria-expanded={isOpen}
              aria-haspopup="listbox"
              aria-autocomplete="list"
              aria-controls={listboxId}
              aria-activedescendant={
                highlightedIndex >= 0 && results[highlightedIndex]
                  ? `student-option-${results[highlightedIndex].id}`
                  : undefined
              }
              aria-busy={isLoading}
              placeholder={placeholder}
              value={searchTerm}
              onKeyDown={handleKeyDown}
              onChange={(e) => {
                const val = e.target.value;
                setSearchTerm(val);
                if (val.trim().length < minChars) {
                  setResults([]);
                  setIsLoading(false);
                  setSearchError(null);
                  setIsOpen(false);
                  setHighlightedIndex(-1);
                } else {
                  setIsLoading(true);
                }
              }}
              onFocus={() => {
                if (searchTerm.trim().length >= minChars && results.length > 0) {
                  setIsOpen(true);
                }
              }}
              disabled={disabled}
              className="pl-9 pr-9 font-normal text-sm"
              autoComplete="off"
            />
            {isLoading && (
              <Loader2 className="absolute right-3 top-1/2 size-4 -translate-y-1/2 animate-spin text-muted-foreground" />
            )}
          </div>

          {/* Screen Reader Live Announcement */}
          <div className="sr-only" aria-live="polite">
            {isLoading
              ? "Searching students..."
              : isOpen && results.length > 0
              ? `${results.length} students found. Use up and down arrow keys to navigate.`
              : isOpen && searchTerm.trim().length >= minChars
              ? "No students found."
              : ""}
          </div>

          {/* Autocomplete Dropdown Listbox */}
          {isOpen && (
            <div
              className="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-md border border-border bg-popover text-popover-foreground shadow-md"
              data-testid="student-search-results"
            >
              {searchError ? (
                <div className="flex items-center gap-2 p-3 text-xs text-destructive">
                  <AlertCircle className="size-4 shrink-0" />
                  <span>{searchError}</span>
                </div>
              ) : results.length === 0 && !isLoading ? (
                <div
                  className="p-4 text-center text-xs text-muted-foreground"
                  data-testid="no-students-found"
                >
                  <p className="font-medium text-foreground">No students found</p>
                  <p className="mt-0.5">
                    No active student matching &ldquo;{debouncedQuery}&rdquo;
                  </p>
                </div>
              ) : (
                <ul
                  ref={listboxRef}
                  id={listboxId}
                  role="listbox"
                  aria-label="Student search results"
                  className="p-1 space-y-0.5"
                >
                  {results.map((student, index) => {
                    const isAssigned = existingStudentIds.includes(student.id);
                    const isHighlighted = highlightedIndex === index;
                    const roll = student.rollNo || student.roll_no;

                    return (
                      <li
                        key={student.id}
                        id={`student-option-${student.id}`}
                        role="option"
                        aria-selected={isHighlighted}
                        aria-disabled={isAssigned}
                        onClick={() => {
                          if (!isAssigned) {
                            handleSelectStudent(student);
                          }
                        }}
                        onMouseEnter={() => setHighlightedIndex(index)}
                        className={`flex items-center justify-between gap-2 rounded-sm px-3 py-2 text-sm transition-colors cursor-pointer select-none ${
                          isAssigned
                            ? "opacity-50 cursor-not-allowed bg-muted/40"
                            : isHighlighted
                            ? "bg-accent text-accent-foreground"
                            : "hover:bg-accent/50 text-foreground"
                        }`}
                      >
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="font-mono text-xs font-semibold px-1.5 py-0.5 rounded bg-muted text-foreground">
                            {roll}
                          </span>
                          <span className="font-medium truncate">{student.name}</span>
                          <span className="text-xs text-muted-foreground truncate">
                            ({student.email})
                          </span>
                        </div>

                        {isAssigned ? (
                          <Badge variant="outline" className="text-[10px] shrink-0 border-amber-500/40 text-amber-600 dark:text-amber-400">
                            Assigned
                          </Badge>
                        ) : isHighlighted ? (
                          <Check className="size-4 shrink-0 text-primary" />
                        ) : null}
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default StudentAutocompleteSelector;
