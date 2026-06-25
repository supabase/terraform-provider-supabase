package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supabase/cli/pkg/api"
)

func sessionsToModel(ctx context.Context, resp *api.AuthConfigResponse) (types.Object, diag.Diagnostics) {
	return buildObj(ctx, SessionsModel{
		Timebox:           NullableToInt64(resp.SessionsTimebox),
		InactivityTimeout: NullableToInt64(resp.SessionsInactivityTimeout),
		SinglePerUser:     NullableToBool(resp.SessionsSinglePerUser),
	})
}

func sessionsToBody(ctx context.Context, sessObj types.Object, body *api.UpdateAuthConfigBody) diag.Diagnostics {
	if sessObj.IsNull() || sessObj.IsUnknown() {
		return nil
	}
	sess, diags := readObj[SessionsModel](ctx, sessObj)
	body.SessionsTimebox = setFloat32(sess.Timebox)
	body.SessionsInactivityTimeout = setFloat32(sess.InactivityTimeout)
	body.SessionsSinglePerUser = setBool(sess.SinglePerUser)
	return diags
}
