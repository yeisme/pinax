package judgment

import "fmt"

// AdoptionBlocker names one required pair that blocks adopting the result as
// a complete evaluation.
type AdoptionBlocker struct {
	CandidateID  string
	QuestionID   string
	AnswerStatus string
	Problem      string // missing | abstained | error | not_answered
}

func (b AdoptionBlocker) String() string {
	return fmt.Sprintf("%s/%s: %s", b.CandidateID, b.QuestionID, b.Problem)
}

// AdoptionBlockers lists every required question pair that is not explicitly
// answered. Any non-empty list means the owner must not present the result as
// a complete, adoptable evaluation; structure is validated before this runs,
// and domain thresholds stay with the owner.
func AdoptionBlockers(req *Request, res *Result) []AdoptionBlocker {
	if req == nil || res == nil {
		return nil
	}
	var blockers []AdoptionBlocker
	for _, question := range req.Questions {
		if !question.Required {
			continue
		}
		for _, candidateID := range question.CandidateIDs {
			item := res.ItemByPair(candidateID, question.QuestionID)
			if item == nil {
				blockers = append(blockers, AdoptionBlocker{
					CandidateID: candidateID, QuestionID: question.QuestionID, Problem: "missing",
				})
				continue
			}
			if item.AnswerStatus != AnswerAnswered {
				blockers = append(blockers, AdoptionBlocker{
					CandidateID: candidateID, QuestionID: question.QuestionID,
					AnswerStatus: item.AnswerStatus, Problem: item.AnswerStatus,
				})
			}
		}
	}
	return blockers
}

// CompleteAdoptionAllowed reports whether every required pair is explicitly
// answered. It does not evaluate domain thresholds or quality.
func CompleteAdoptionAllowed(req *Request, res *Result) bool {
	return len(AdoptionBlockers(req, res)) == 0
}
