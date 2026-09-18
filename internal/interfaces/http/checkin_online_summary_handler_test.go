package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	checkinapp "SWPUCAT/internal/application/checkin"
	checkindomain "SWPUCAT/internal/domain/checkin"
	userdomain "SWPUCAT/internal/domain/user"

	"github.com/gin-gonic/gin"
)

type onlineSummaryHandlerCheckins struct {
	checkindomain.Repository
	online map[int64]bool
}

func (r *onlineSummaryHandlerCheckins) GetLastByUserAndDate(_ context.Context, userID int64, date string) (*checkindomain.CheckinRecord, error) {
	if !r.online[userID] {
		return nil, nil
	}
	return &checkindomain.CheckinRecord{UserID: userID, Type: checkindomain.CheckinTypeIn, Date: date}, nil
}

type onlineSummaryHandlerUsers struct {
	userdomain.Repository
	members []*userdomain.User
}

func (r *onlineSummaryHandlerUsers) FindAll(context.Context) ([]*userdomain.User, error) {
	return r.members, nil
}

func TestGetAgentOnlineSummaryAppliesRoleBasedProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := checkinapp.NewCheckinService(
		&onlineSummaryHandlerCheckins{online: map[int64]bool{7: true, 8: true}},
		&onlineSummaryHandlerUsers{members: []*userdomain.User{
			{ID: 7, Nickname: userdomain.Nickname("当前成员")},
			{ID: 8, Nickname: userdomain.Nickname("其他成员")},
		}},
		nil,
		nil,
	)
	handler := NewCheckinHandler(service, nil)

	for _, test := range []struct {
		name      string
		role      string
		wantNames bool
	}{
		{name: "member", role: "member", wantNames: false},
		{name: "captain", role: "captain", wantNames: true},
		{name: "super_admin", role: "super_admin", wantNames: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/checkin/online-summary", nil)
			ctx.Set(ContextUserID, int64(7))
			ctx.Set(ContextRole, test.role)
			handler.GetAgentOnlineSummary(ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			var envelope struct {
				Code int                         `json:"code"`
				Data checkinapp.OnlineSummaryDTO `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != 0 || envelope.Data.OnlineCount != 2 || envelope.Data.SelfOnline == nil || !*envelope.Data.SelfOnline {
				t.Fatalf("response = %+v", envelope)
			}
			if test.wantNames {
				if len(envelope.Data.OnlineMembers) != 2 {
					t.Fatalf("manager names = %#v", envelope.Data.OnlineMembers)
				}
			} else if envelope.Data.OnlineMembers != nil {
				t.Fatalf("member received names = %#v", envelope.Data.OnlineMembers)
			}
			for _, forbidden := range []string{"user_id", "token", "ip", "device", "activity"} {
				if json.Valid(recorder.Body.Bytes()) && containsJSONText(recorder.Body.String(), forbidden) {
					t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func containsJSONText(value, needle string) bool {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
