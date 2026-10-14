package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSM_Transitions(t *testing.T) {
	tests := []struct {
		name        string
		current     Status
		event       Event
		expected    Status
		expectErr   bool
		expectedErr error
	}{
		// PENDING / NOT_STARTED
		{
			name:      "PENDING + BEGIN -> IN_PROGRESS",
			current:   StatusPending,
			event:     EventBegin,
			expected:  StatusInProgress,
			expectErr: false,
		},
		{
			name:      "NOT_STARTED + BEGIN -> IN_PROGRESS",
			current:   StatusNotStarted,
			event:     EventBegin,
			expected:  StatusInProgress,
			expectErr: false,
		},
		{
			name:        "PENDING + SUBMIT -> error",
			current:     StatusPending,
			event:       EventSubmit,
			expectErr:   true,
			expectedErr: ErrInvalidTransition,
		},
		{
			name:        "PENDING + RECORD_ANSWER -> error",
			current:     StatusPending,
			event:       EventRecordAnswer,
			expectErr:   true,
			expectedErr: ErrInvalidTransition,
		},
		{
			name:        "NOT_STARTED + NAVIGATE_NEXT -> error",
			current:     StatusNotStarted,
			event:       EventNavigateNext,
			expectErr:   true,
			expectedErr: ErrInvalidTransition,
		},

		// IN_PROGRESS
		{
			name:      "IN_PROGRESS + RECORD_ANSWER -> IN_PROGRESS",
			current:   StatusInProgress,
			event:     EventRecordAnswer,
			expected:  StatusInProgress,
			expectErr: false,
		},
		{
			name:      "IN_PROGRESS + NAVIGATE_NEXT -> IN_PROGRESS",
			current:   StatusInProgress,
			event:     EventNavigateNext,
			expected:  StatusInProgress,
			expectErr: false,
		},
		{
			name:      "IN_PROGRESS + NAVIGATE_PREVIOUS -> IN_PROGRESS",
			current:   StatusInProgress,
			event:     EventNavigatePrevious,
			expected:  StatusInProgress,
			expectErr: false,
		},
		{
			name:      "IN_PROGRESS + SUBMIT -> SUBMITTED",
			current:   StatusInProgress,
			event:     EventSubmit,
			expected:  StatusSubmitted,
			expectErr: false,
		},
		{
			name:      "IN_PROGRESS + SYSTEM_EXPIRE -> EXPIRED",
			current:   StatusInProgress,
			event:     EventSystemExpire,
			expected:  StatusExpired,
			expectErr: false,
		},
		{
			name:        "IN_PROGRESS + BEGIN -> error",
			current:     StatusInProgress,
			event:       EventBegin,
			expectErr:   true,
			expectedErr: ErrInvalidTransition,
		},

		// SUBMITTED (terminal)
		{
			name:        "SUBMITTED + SUBMIT -> already submitted",
			current:     StatusSubmitted,
			event:       EventSubmit,
			expectErr:   true,
			expectedErr: ErrSessionAlreadySubmitted,
		},
		{
			name:        "SUBMITTED + RECORD_ANSWER -> error",
			current:     StatusSubmitted,
			event:       EventRecordAnswer,
			expectErr:   true,
			expectedErr: ErrInvalidTransition,
		},
		{
			name:        "SUBMITTED + NAVIGATE_NEXT -> error",
			current:     StatusSubmitted,
			event:       EventNavigateNext,
			expectErr:   true,
			expectedErr: ErrInvalidTransition,
		},

		// EXPIRED (terminal)
		{
			name:        "EXPIRED + SYSTEM_EXPIRE -> session expired",
			current:     StatusExpired,
			event:       EventSystemExpire,
			expectErr:   true,
			expectedErr: ErrSessionExpired,
		},
		{
			name:        "EXPIRED + RECORD_ANSWER -> session expired",
			current:     StatusExpired,
			event:       EventRecordAnswer,
			expectErr:   true,
			expectedErr: ErrSessionExpired,
		},
		{
			name:        "EXPIRED + SUBMIT -> session expired",
			current:     StatusExpired,
			event:       EventSubmit,
			expectErr:   true,
			expectedErr: ErrSessionExpired,
		},
		{
			name:        "EXPIRED + NAVIGATE_NEXT -> session expired",
			current:     StatusExpired,
			event:       EventNavigateNext,
			expectErr:   true,
			expectedErr: ErrSessionExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := Transition(tt.current, tt.event)
			if tt.expectErr {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, next)
			}
		})
	}
}
