package controllers

import (
	"fmt"
	"strings"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ListSequences devolve as cadências com inscritos e as taxas de abertura e
// resposta — as colunas que a equipe usa para comparar sequências.
func ListSequences(ctx iris.Context) {
	list, err := models.ListSequences(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": list, "steps": models.StepLabels})
}

func GetSequence(ctx iris.Context) {
	seq, err := models.SequenceByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(seq)
}

type sequenceRequest struct {
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	Steps         []models.SequenceStep `json:"steps"`
	Active        *bool                 `json:"active"`
	Dynamic       bool                  `json:"dynamic"`
	ExitOnReply   *bool                 `json:"exit_on_reply"`
	ExitOnMeeting *bool                 `json:"exit_on_meeting"`
	OwnerID       *int64                `json:"owner_id"`
}

func (r *sequenceRequest) apply(seq *models.Sequence) string {
	seq.Name = r.Name
	seq.Description = strings.TrimSpace(r.Description)
	seq.Steps = r.Steps
	seq.Dynamic = r.Dynamic
	seq.OwnerID = r.OwnerID

	seq.Active = true
	if r.Active != nil {
		seq.Active = *r.Active
	}
	// As duas regras de saída vêm ligadas: é o comportamento que a equipe espera.
	seq.ExitOnReply = true
	if r.ExitOnReply != nil {
		seq.ExitOnReply = *r.ExitOnReply
	}
	seq.ExitOnMeeting = true
	if r.ExitOnMeeting != nil {
		seq.ExitOnMeeting = *r.ExitOnMeeting
	}

	if err := models.ValidateSequence(seq); err != nil {
		return err.Error()
	}
	return ""
}

func CreateSequence(ctx iris.Context) {
	var req sequenceRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	seq := &models.Sequence{}
	if msg := req.apply(seq); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if claims := middlewareClaims(ctx); claims != nil {
		seq.CreatedBy = &claims.UserID
		if seq.OwnerID == nil {
			seq.OwnerID = &claims.UserID
		}
	}

	if err := models.CreateSequence(utils.DB, seq); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditCreate, "sequencia", seq.ID, "criou a sequência "+seq.Name)
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(seq)
}

func UpdateSequenceByID(ctx iris.Context) {
	seq, err := models.SequenceByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req sequenceRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.apply(seq); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if err := models.UpdateSequence(utils.DB, seq); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "sequencia", seq.ID, "editou a sequência "+seq.Name)
	ctx.JSON(seq)
}

func DeleteSequenceByID(ctx iris.Context) {
	id := paramID(ctx)
	seq, err := models.SequenceByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteSequence(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "sequencia", id, "excluiu a sequência "+seq.Name)
	ctx.JSON(iris.Map{"message": "sequência removida"})
}

// ListSequenceMembers devolve os inscritos e em que etapa cada um parou.
func ListSequenceMembers(ctx iris.Context) {
	members, err := models.ListMembers(utils.DB, paramID(ctx), ctx.URLParam("status"))
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": members})
}

// EnrollInSequence inscreve um ou mais contatos na cadência.
func EnrollInSequence(ctx iris.Context) {
	sequenceID := paramID(ctx)
	seq, err := models.SequenceByID(utils.DB, sequenceID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if !seq.Active {
		badRequest(ctx, "a sequência está pausada: ative antes de inscrever contatos")
		return
	}

	var req struct {
		ContactIDs []int64 `json:"contact_ids"`
	}
	if err := ctx.ReadJSON(&req); err != nil || len(req.ContactIDs) == 0 {
		badRequest(ctx, "selecione ao menos um contato")
		return
	}
	if len(req.ContactIDs) > 200 {
		badRequest(ctx, "no máximo 200 contatos por vez")
		return
	}

	var enrolledBy *int64
	if claims := middlewareClaims(ctx); claims != nil {
		enrolledBy = &claims.UserID
	}

	inscritos, recusados := 0, []string{}
	for _, contactID := range req.ContactIDs {
		if err := models.EnrollContact(utils.DB, sequenceID, contactID, enrolledBy); err != nil {
			recusados = append(recusados, err.Error())
			continue
		}
		inscritos++
	}

	audit(ctx, models.AuditUpdate, "sequencia", sequenceID,
		fmt.Sprintf("inscreveu %d contato(s) na sequência %s", inscritos, seq.Name))
	ctx.JSON(iris.Map{"enrolled": inscritos, "skipped": recusados})
}

// UnenrollFromSequence tira o contato da cadência a pedido de uma pessoa.
func UnenrollFromSequence(ctx iris.Context) {
	memberID := ctx.Params().GetInt64Default("memberId", 0)

	member, err := models.MemberByID(utils.DB, memberID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.FinishMember(utils.DB, memberID, models.MemberExited, "manual"); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "sequencia", member.SequenceID,
		"tirou "+member.ContactName+" da sequência")
	ctx.JSON(iris.Map{"message": "contato removido da sequência"})
}

// ContactSequences mostra em quais cadências o contato está — usado na ficha
// dele para não inscrever duas vezes.
func ContactSequences(ctx iris.Context) {
	contactID := ctx.URLParamInt64Default("contact_id", 0)
	if contactID == 0 {
		badRequest(ctx, "informe o contact_id")
		return
	}

	rows, err := utils.DB.Query(`
		SELECT m.id, m.sequence_id, s.name, m.step, m.status, m.enrolled_at
		FROM sequence_members m
		JOIN sequences s ON s.id = m.sequence_id
		WHERE m.contact_id = $1
		ORDER BY m.enrolled_at DESC`, contactID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	defer rows.Close()

	type item struct {
		ID         int64     `json:"id"`
		SequenceID int64     `json:"sequence_id"`
		Name       string    `json:"name"`
		Step       int       `json:"step"`
		Status     string    `json:"status"`
		EnrolledAt time.Time `json:"enrolled_at"`
	}
	list := []item{}
	for rows.Next() {
		var i item
		if err := rows.Scan(&i.ID, &i.SequenceID, &i.Name, &i.Step, &i.Status, &i.EnrolledAt); err != nil {
			serverError(ctx, err)
			return
		}
		list = append(list, i)
	}
	ctx.JSON(iris.Map{"data": list})
}
